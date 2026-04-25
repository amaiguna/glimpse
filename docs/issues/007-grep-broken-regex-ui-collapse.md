# Issue #007: grep モードで不正 regex 入力時に UI が崩壊する

## 現状

grep モードで textinput に不正な正規表現（`[`, `(?`, `[unclosed` 等）を入力すると、画面全体が崩壊し、`error: exit status 2` という意味不明な 1 行だけが残る。textinput / 結果リスト / preview ペインなどの通常レイアウトはすべて消える。

### 再現手順

1. `glimpse` を起動して grep モードに切り替え
2. クエリ欄に `[` を 1 文字打つ
3. ペイン枠ごと表示が `error: exit status 2` だけに置き換わる
4. バックスペースで `[` を消すと表示は復帰するが、入力中は何も見えない

### 内部フロー

1. textinput → debounce → `runGrepCmd(ctx, "[")` (`internal/ui/grep_model.go:57`)
2. `grep.Search` → `exec.CommandContext(ctx, rg, "--json", "[")` (`internal/grep/grep.go:129`)
3. rg は **exit code 2** + 空 stdout + stderr に `regex parse error: ...` を出力
4. `runWithLimit` は stdout のみ捕捉、stderr は `/dev/null` 行き（`cmd.Stderr` 未設定 — #008 参照）
5. `Search` の error 分岐は exit code 1（マッチなし）のみ特別扱い、2 は素通り (`grep.go:140`)
6. `runGrepCmd` が `GrepErrorMsg{Err: *exec.ExitError("exit status 2")}` を返す
7. `g.err = msg.Err` がセットされ、`model.go:311-312` の早期 return で View 全崩壊（#009 参照）

### 観測される問題

- **メッセージが `exit status 2` だけで原因不明**（#008 起因）
- **入力途中の中間状態が常時エラー**: `[abc]` を打つ過程の `[`, `[a`, `[ab` はすべて不正 regex なので debounce ごとに崩壊が発生
- **修正したい瞬間に視覚フィードバックが消える**（#009 起因）

## 対応方針の選択肢

### A. exit code 2 を「マッチなし」相当に扱う

`grep.Search` 内で exit code 2 を nil 結果として返す。UI は崩壊しなくなるが、stderr の診断情報は引き続き失われる。最も低コスト。

### B. exit code 2 を専用 Msg で UI に伝える

`GrepRegexErrorMsg` 等の型を導入し、UI 側は通常レイアウトを維持したまま textinput 直下のステータス行に regex エラーメッセージを表示する。本体ペインは前回の検索結果を維持して入力継続を妨げない。

- 前提条件: stderr の捕捉（#008）と pane エラー時 View の段階的レンダリング（#009）

### C. クライアント側 regex 事前検証

`regexp/syntax` で打鍵時点で検証し、不正なら rg を起動しない。但し rg は Rust regex 方言なので Go の `regexp` と完全一致せず、誤判定の可能性あり。

## 判断

**B を採用**。ただし #008（stderr 捕捉）と #009（pane.Err 時の View 動作）の解決が前提。これらが先に完了すれば、本 issue の修正は

- `grep.Search` が exit code 2 + stderr メッセージを `GrepRegexErrorMsg` 相当の構造化エラーで返す
- `GrepModel` がそれを `g.regexErr` のような専用フィールドに格納
- View が textinput 下に 1 行で表示

という形に収束する。A 単独だとエラー診断が永久に失われるので不採用。C は方言差リスクで不採用。

## 関連 issue

- #008 外部コマンドの stderr 喪失でエラー診断が無効化（前提条件）
- #009 pane.Err() が真の時に View を全崩壊させる UX（前提条件）

## 優先度

**高** — TUI の核機能（grep 入力中）で常時発生する UX 重大悪化。grep モードを使い始めて 1 文字目から潜在的に発火する。
