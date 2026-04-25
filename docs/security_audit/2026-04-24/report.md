# セキュリティ監査レポート (2026-04-24)

**Status: Closed (2026-04-25)**

`go-cli-security-audit` skill のワークフロー（Phase 1〜4）に基づき実施。

- 対象コミット: `cf8e186` (main)
- Go バージョン: `go1.26.2-X:nodwarf5 linux/amd64`
- 対象範囲: プロジェクト全体（`./...`）
- High 項目の再現手順: [`repro/README.md`](repro/README.md)

## クロージングノート (2026-04-25)

本監査の全 13 件（High×3 / Medium×3 / Low×3 / Info×4）と Phase 4 の Fuzz 追加 2 件について、対応または評価判断が完了したためクローズする。

- **対応済 (12件)**: H-1, H-2, H-3, M-1, M-2, M-3, L-2, L-3, I-1, I-2, I-3, I-4
- **Won't-fix (1件)**: L-1（symlink）— 脅威モデル評価の結果。`cat` と同等で glimpse 固有の攻撃面ではなく、UX 影響も大きいため。次回監査時に fd/rg のデフォルト挙動変更や `--follow` 相当の独自有効化があれば再評価する。
- **Phase 4 Fuzz**: `FuzzForTerminal` / `FuzzParseGrepItem` / `FuzzReadFileRange` 追加完了。`FuzzParseGrepItem` は実装中に `:00` 入力での line==0 曖昧バグを 1 件検出し、parse 仕様を 1-based 強制に修正。

最終確認: `go test ./...` / `go test -tags=integration ./...` / `go vet ./...` / `staticcheck ./...` いずれも無指摘。

本ディレクトリは以降 immutable とする。再発見・再評価が必要な項目は次回監査の新ディレクトリで取り扱う（[`docs/security_audit/README.md`](../README.md) 参照）。

---

## サマリ

| 優先度 | 件数 | 概要 | ステータス |
|---|---|---|---|
| High | 3 | ターミナルエスケープシーケンス注入（preview/ファイル名/grep 行） | ✅ 対応済（2026-04-24） |
| Medium | 3 | preview OOM、エディタ引数フラグ注入、`exec.CommandContext` 不使用 | ✅ 対応済（2026-04-25） |
| Low | 3 | symlink、PATH 汚染、環境変数無制限継承 | L-2/L-3 ✅ 対応済（2026-04-25）／L-1 Won't-fix |
| Info | 4 | stdout サイズ未制限、`parseGrepItem` 境界、非推奨 API、未使用コード | ✅ 対応済（2026-04-25） |

依存関係・メモリモデルはクリーン（`govulncheck` / `osv-scanner` / `go test -race` すべて無指摘）。
最大リスクは **TUI 特有のターミナルエスケープ注入**で、汎用ツールでは検出できない観点。High 3件は共通サニタイザ `internal/sanitize` の導入で解消。Medium 3件も 2026-04-25 時点で対応完了（preview サイズ上限 2MB、エディタ引数 `--` セパレータ + `./` 前置、`exec.CommandContext` 化 + デバウンスキャンセル）。Low のうち L-2（PATH 汚染）/ L-3（env 無制限継承）も同日対応済。L-1（symlink）は脅威モデル評価の結果 Won't-fix。Info 4件（stdout サイズ上限、`parseGrepItem` rsplit 化、`tea.MouseLeft` 非推奨 API 解消、未使用コード削除）も同日対応済で、`staticcheck` は無指摘。

---

## Phase 1: 自動静的解析

| ツール | 結果 |
|---|---|
| `govulncheck ./...` | No vulnerabilities |
| `osv-scanner --lockfile go.mod` | No issues (32 packages) |
| `go test -race ./...` | All pass |
| `gosec ./...` | 6件（G204 × 3, G702 × 1, G304 × 3 ― いずれも Phase 3 で判定） |
| `staticcheck ./...` | 2件（`SA1019` 非推奨 API, `U1000` 未使用コード） |

`gosec` の指摘は以下を除いて誤検知ではなく、Phase 3 の文脈で扱う:

- `G204 exec.Command("rg","--json",pattern)` — コマンド名はハードコード、引数のみユーザー入力。シェル経由ではないので真のコマンドインジェクションは不可。ただし `--` セパレータ不足は Medium。
- `G304 os.ReadFile(path)` — path は fd/rg が返したファイル一覧内のみ。ユーザー自身がアクセスできるファイル範囲なので権限昇格はなし。ただし preview の OOM は別論点（Medium）。

---

## Phase 2: 脅威モデリング

### 流入点 → Sink マップ

| 流入点 | 経由 | Sink |
|---|---|---|
| `EDITOR` env | そのまま | `exec.Command(editor, ...)` (`internal/ui/model.go:287`) |
| textinput クエリ | そのまま | `exec.Command("rg","--json",pattern)` (`internal/grep/grep.go:50`) |
| fd/rg の出力ファイル名 | `items` 配列 | **描画**（`View`）／`exec.Command(editor, file)` |
| `rg --json` の `lines.text` | `items` 配列 | **描画**（ファイル内容由来の制御文字を含みうる） |
| ファイル内容 | `os.ReadFile` → chroma | **描画**（ANSI エスケープ残存） |
| ファイルパス | fd/rg 由来 | `os.ReadFile` / `os.Open` |

### 攻撃者モデル

- **ローカルユーザーの自攻撃**: `EDITOR` 書き換え、`PATH` 汚染、`rg`/`fd` 差し替え。影響はユーザー自身の権限範囲のみ。
- **悪意あるファイルを配置した第三者**: 被害者がそのディレクトリで glimpse を起動すると、ファイル名・ファイル内容・grep 結果経由で**画面乗っ取り／タイトル書換／クリップボード書き込み（OSC 52）／一部ターミナルでは RCE** に至る可能性。TUI fuzzy finder では最重要の攻撃経路。

---

## Phase 3: 発見事項

### High

#### H-1: ファイル内容経由のターミナルエスケープ注入 ✅ 対応済

- 該当箇所: `internal/preview/preview.go:43, 65` → `internal/ui/model.go:334-343` で描画
- 内容: `chroma` は入力テキスト中の `\x1b[…` を除去しない。`ansi.Truncate` は本物の ANSI と区別できないのでそのまま保持する。ファイル内容に制御シーケンスを含ませた悪意あるファイルを preview に表示しただけで、タイトルバー書換・画面クリア+偽プロンプト表示・OSC 52 クリップボード書き込みなどが可能。
- 対策: `os.ReadFile` の結果を chroma に渡す**前**に制御文字サニタイズを適用する。
- 実装: `internal/preview/preview.go` の `ReadFile` / `ReadFileRange` で読み込み直後に `sanitize.ForTerminal` を適用。`Highlight` のエラーフォールバック経路（`internal/ui/model.go`）でも raw が表示されないよう、reader 層でサニタイズする方針を採用。回帰テスト: `TestReadFileSanitizesEscapes`, `TestReadFileRangeSanitizesEscapes`。

#### H-2: ファイル名経由のターミナルエスケープ注入 ✅ 対応済

- 該当箇所: `internal/finder/finder.go:8-22`（fd/rg 出力）→ `internal/ui/finder.go:155-182` 描画
- 内容: ファイル名には改行以外のほぼ全バイトを含めることができ、`fd` / `rg --files` はデフォルトでそれらを出力する。悪意ある第三者が `$'\x1b[2J\x1b[H偽プロンプト'` のようなファイルを置いたディレクトリで被害者が glimpse を起動すると、左ペイン描画時に画面が乗っ取られる。
- 対策: View 前に同サニタイザを通す。
- 実装: `internal/ui/finder.go` の `View()` 内で `sanitize.ForTerminal(item)` → `truncateToWidth` の順に適用。`SelectedItem` / `FilePath` / `OpenTarget` は `os.ReadFile` / `exec.Command` 用に raw を返す（描画と用途が異なるため `items` 自体は無加工で保持）。回帰テスト: `TestFinderViewSanitizesEscapesInFilenames`, `TestFinderRawPathsForOperations`。

#### H-3: grep 行内容経由のターミナルエスケープ注入 ✅ 対応済

- 該当箇所: `internal/grep/grep.go:86`（`rg --json` の `lines.text` を格納）→ `internal/ui/grep_model.go:174-180` 描画
- 内容: `rg --json` の行テキストは生ファイル内容。H-1 と同じ攻撃面が grep モードでも成立。
- 対策: `formatGrepMatches` の時点、あるいは View 直前にサニタイズ。
- 実装: `internal/ui/grep_model.go` の `View()` 内で `parseGrepItem` 抽出後の `displayItem`（=ファイルパス）に `sanitize.ForTerminal` を適用。Grep モードの View はファイル名のみを表示するためファイル名経路が攻撃面の中心であり、ファイル内容のヒット行は preview 側（H-1 で防御済）で表示される。raw な `items` は `OpenTarget` / `FilePath` 用に保持。回帰テスト: `TestGrepViewSanitizesEscapesInFilenames`, `TestGrepRawPathsForOperations`。

#### 共通対策: サニタイザ実装

`internal/sanitize/sanitize.go` に `ForTerminal` を新規実装した。下記の点で当初提案より強化している:

- `unicode.In(r, unicode.Cc, unicode.Cf)` を使用し、BiDi 制御文字（U+202A〜202E, U+2066〜2069 等）も `\uNNNN` 形式で可視化（**Trojan Source / CVE-2021-42574 対策**）。`golang.org/x/text/unicode/bidi` への依存追加は不要。
- 不正 UTF-8 バイトを `utf8.DecodeRuneInString` で検出し `\xNN` で可視化（生バイトを描画に流さない）。
- 改行 `\n` とタブ `\t` のみホワイトリストとして通し、それ以外の C0/C1/DEL を全可視化。

テスト戦略（`internal/sanitize/sanitize_test.go`）:

- **テーブル駆動**で OSC 0/52/8、SGR、画面クリア、BEL/NUL/DEL、BiDi RLO/LRO/RLI、C1 制御、不正 UTF-8 を網羅（20ケース）。
- **冪等性プロパティ** `TestForTerminalIdempotent`（`ForTerminal(ForTerminal(s)) == ForTerminal(s)`）。
- **不変条件 fuzz** `FuzzForTerminal`: 任意バイト列入力に対し、出力に ESC / DEL / 改行タブ以外の C0 / BiDi 制御 が含まれず、UTF-8 として valid、かつ冪等であること。100K execs 無 panic。

### Medium

#### M-1: preview の無制限メモリ読み込み (OOM) ✅ 対応済

- 該当箇所: `internal/preview/preview.go:43, 65`
- 内容: `os.ReadFile` はファイル全体を一度にメモリへ読み込む。`ReadFileRange` も全読み込み後に行スライスで切る実装のため、サイズ上限がない。GB 級のログファイルや動画をカーソルで選択しただけで即 OOM。
- 対策: 先頭 Nバイト（例: 1〜4MB）で打ち切る、あるいは `bufio.Scanner` を使った行単位 streaming 読み。サイズ上限は `binarySniffSize` と整合を取った定数で管理するのが望ましい。
- 実装: `internal/preview/preview.go` に `MaxPreviewSize = 2 * 1024 * 1024` と `LargeFileMessage` / `IsTooLarge(path)` を追加。`internal/ui/model.go` の `previewCmd` で `IsTooLarge` を `IsBinary` より先にチェックし、超過時は `LargeFileMessage` を返す（stat のみで判定できるため安価）。2MB の根拠は「通常ソースファイルは数KB〜100KB 台に収まり、この閾値を超えるのは minified bundle / `package-lock.json` / generated 系が中心 = 人間が読むテキストではない」。バイナリは既存の `IsBinary`（NUL sniff）で別経路。回帰テスト: `TestIsTooLarge`（境界条件 table-driven）、`TestMaxPreviewSizeIs2MB`（定数意図ロック）、`TestPreviewLargeFile{In,InGrep}Mode`、`TestPreviewExactlyMaxSizeIsAllowed`。golden: `preview_binary_message`, `preview_large_file_message`（状態メッセージ描画レイアウト pin、BinaryFileMessage 既存ギャップも同時に埋めた）。

#### M-2: エディタ起動時の `--` セパレータ欠如 ✅ 対応済

- 該当箇所: `internal/ui/model.go:286-287`
- 内容: `args = append(args, fmt.Sprintf("+%d", line)); args = append(args, file)` でエディタに引数を渡しているが `--` なし。細工ファイル名 `-c:set shell=...` のようなファイルを Enter で開くと Vim/Neovim が引数扱いして**任意コマンド実行**に繋がる。
- 対策: `args = append(args, "--", file)` を追加。エディタ別に `--` サポート差があるので、`vim`/`nvim`/`emacs`/`code` など主要エディタの互換を確認する。
- 実装: `internal/ui/editor.go` に `buildEditorArgs(editor, file, line)` を抽出し、エディタ別にディスパッチ:
  - `vim` / `nvim` / `emacs` / `vi` / 未知: `+LINE -- FILE`（`--` で option 終端）
  - `code` / `code-insiders` / `codium` / `vscodium`: `-g FILE:LINE`（`-g` が次トークンを値として消費する性質に依存）
  - `zed`: `-- FILE:LINE`（positional）

  二層防御として、`sanitizeEditorFilePath` がファイル名先頭の `-` `+` を検出した場合は `./` を前置し、パーサーの差異に依らずフラグと lexically に区別できる形にする。回帰テスト: 23 ケースの table-driven `TestBuildEditorArgs` + `TestBuildEditorArgsNoFlagShaped`（ユーザー由来トークンが argv にフラグ形状で残らない不変条件プロパティ）。

#### M-3: `exec.CommandContext` 不使用 ✅ 対応済

- 該当箇所: `internal/grep/grep.go:50`, `internal/finder/finder.go:9, 12`
- 内容: `exec.Command` のみで context を渡していない。デバウンス中にキーストローク毎に rg が立ち上がるが、古いプロセスはキャンセルできず stdout がメモリに溜まる。暴走 rg を殺せない。
- 対策: `context.WithTimeout` + `exec.CommandContext`。クエリ変更時に前回 context をキャンセルできる構造にする。
- 実装: `grep.Search(ctx, pattern)` / `finder.ListFiles(ctx)` にシグネチャ変更し `exec.CommandContext` 化。`ctx.Err()` を `*exec.ExitError` より優先して返し、rg の exit code 1（マッチなし）とキャンセルを区別する。`GrepModel` に `searchCancel context.CancelFunc` フィールドを追加し、`handleDebounceTick` 発火時に前回 cancel → 新 ctx（10s timeout）作成、`Reset` 時も cancel して進行中の rg を回収。`runGrepCmd` は `context.Canceled` を握りつぶし（新検索で上書き中のため UI に「killed」エラーを出さない）、`DeadlineExceeded` は `GrepErrorMsg` として表示。`loadFilesCmd` は一回限りなので 30s timeout + `defer cancel()`。回帰テスト: `TestGrepCancelsPreviousSearchOnNewDebounce` / `TestGrepCancelsSearchOnReset`（spy cancel 注入で状態遷移検証）。integration テスト（`//go:build integration`）: `TestSearchContextCanceled` / `TestSearchContextDeadlineExceeded` / `TestSearchNormalContext`、`TestListFilesContextCanceled` / `TestListFilesNormalContext`（実 rg/fd プロセスへの伝播確認）。

### Low

#### L-1: symlink 経由の任意ファイル参照 ⚠️ Won't-fix（2026-04-25）

- 該当箇所: `internal/preview/preview.go:25, 43, 65`
- 内容: `fd --type f` は regular file のみなので symlink は一覧に出ないが、`rg --files` はデフォルトで symlink を追従しない反面 `--follow` 等で挙動が変わる。ユーザー自身の権限範囲内なので escalation はないが、`~/.ssh/id_rsa` などへのうっかり参照は観点として記録しておく。
- 対策: `os.Lstat` で symlink 検知 → skip、あるいはパスが意図したベースディレクトリ以下か検証（Go 1.24+ なら `os.Root`）。
- **判断**: Won't-fix。理由:
  1. **Privilege escalation はゼロ**。ユーザー自身が読めるファイルしか到達できない（`/etc/shadow` 等は権限で弾かれる）。
  2. **glimpse 固有の攻撃面ではない**。最も現実的なのは「未信頼 repo を clone してレビュー中に細工 symlink を選択 → preview に鍵が出る」だが、`cat README.md` でも同じ事が起こるため glimpse が新たなリスクを生んでいるわけではない。
  3. **既存のデフォルト挙動で十分軽減されている**。`fd --type f` は symlink を出さず、`rg --files` も `--follow` 無しではフォローしない。
  4. **ベース外参照を弾く（option 3: `os.Root`）案は UX を破壊**する。プロジェクト外のファイルや home 配下を fuzzy finder で見るのは正常ユースケース。
  5. **`os.Lstat` で skip する案（option 2）も**、symlink を意図的に置いている開発者ワークフロー（`node_modules/.bin` 等）を壊す副作用がある。
  - 将来 fd/rg のデフォルト挙動が変わる、あるいは `--follow` 相当を glimpse が独自に有効化する場合は再評価する。

#### L-2: PATH 汚染対策なし ✅ 対応済

- 該当箇所: `internal/finder/finder.go:9, 12`, `internal/grep/grep.go:50`
- 内容: `exec.Command("rg", ...)` / `("fd", ...)` で PATH 依存解決。カレント実行ディレクトリ直下に悪意ある `rg`/`fd` があると（Go 1.19+ は `.` を PATH から除外するので影響は小さいが）、PATH 上の順序次第では差し替え可能。
- 対策: 起動時に `exec.LookPath` で絶対パス解決し、以降はそれを使う。任意では解決先が `/usr/bin` 等の想定ディレクトリ下かを検証。
- 実装: `internal/grep/grep.go` / `internal/finder/finder.go` でパッケージレベル `var rgBinary = lookupBinary("rg")` / `fdBinary = lookupBinary("fd")` として `exec.LookPath` を起動時に一度だけ実行し、絶対パスで固定。バイナリが見つからない場合は空文字列となり、`Search` / `ListFiles` 側で明示エラーを返す。実行時の相対名解決を排除することで mid-session の PATH 差し替えに対する余地を消す。回帰テスト: `TestRgBinaryResolvedToAbsolutePath`（grep / finder 両方）、`TestFdBinaryResolvedToAbsolutePath`、`TestSearchReturnsErrorWhenBinaryMissing`、`TestListFilesReturnsErrorWhenBothBinariesMissing`。

#### L-3: 子プロセスへの環境変数無制限継承 ✅ 対応済

- 該当箇所: 全 `exec.Command` 呼び出し
- 内容: `cmd.Env` 未設定なので親プロセスの全環境変数（`GIT_SSH_COMMAND`, `LD_PRELOAD`, クレデンシャル系）が rg/fd/editor に流れる。
- 対策: 必要なものだけホワイトリスト（`PATH`, `HOME`, `LANG`, `TERM`, `EDITOR` 依存分）。
- 実装: `internal/grep/grep.go` / `internal/finder/finder.go` に `whitelistedEnv()` を追加し、`PATH` / `HOME` / `LANG` / `LC_ALL` / `LC_CTYPE` / `LC_MESSAGES` のみ通す。`Search` の rg、`ListFiles` の fd / rg 両方で `cmd.Env = whitelistedEnv()` を適用。スコープ判断: エディタ（`internal/ui/model.go` の `openEditorCmd`）は GUI 統合に必要な多数の env（`DISPLAY`, `XDG_*`, `WAYLAND_DISPLAY` 等）に依存するため対象外（option A）。攻撃面としては、rg/fd は単純な検索バイナリなので最小 env で十分機能する一方、エディタは UX を壊さずに env を絞るのが難しい。回帰テスト: `TestWhitelistedEnv`（grep / finder 両方）— `LD_PRELOAD` / `GIT_SSH_COMMAND` / `AWS_SECRET_ACCESS_KEY` が落ちることを `t.Setenv` で確認。

### Info

| ID | 該当箇所 | 内容 | 対応 |
|---|---|---|---|
| I-1 | `internal/finder/finder.go`, `internal/grep/grep.go` | `cmd.Output()` が stdout 全体をメモリ化。巨大リポの `rg --files` は数百MB可能。`StdoutPipe` + `io.LimitReader` が望ましい | ✅ 対応済 |
| I-2 | `internal/ui/grep_model.go:297` | `parseGrepItem` は `:` で 2分割。Windows パスや `:` を含むファイル名で壊れる（panic はしない） | ✅ 対応済 |
| I-3 | `internal/ui/fuzz_test.go:140` | `tea.MouseLeft` 非推奨 (SA1019) → `MouseAction` + `MouseButton` | ✅ 対応済 |
| I-4 | `internal/ui/styles.go:143` | 未使用 `loadingStyle` (U1000) | ✅ 対応済 |

#### I-1: stdout サイズ上限 ✅ 対応済（2026-04-25）

`internal/grep/grep.go` / `internal/finder/finder.go` 両方に `MaxCmdOutputSize = 50 * 1024 * 1024`、`ErrOutputTooLarge`、`readLimited(io.Reader, int64)`、`runWithLimit(*exec.Cmd, int64)` を追加。`cmd.Output()` を `runWithLimit`（`StdoutPipe + io.LimitReader(max+1) + io.ReadAll + Wait`）に置き換え、上限超過時は `ErrOutputTooLarge` を返して残り stdout を `io.Discard` に流して Wait deadlock を回避。`readLimited` は table-driven テストで境界（empty / within / exact / over+1 / much-larger / zero-limit）と Reader エラー伝搬をカバー。`runWithLimit` は実プロセス起動を伴うため統合テスト相当だが、既存の grep / finder の整合テスト（exit 1 = no match の保持、ctx cancel/deadline の扱い）で間接的に検証。

#### I-2: parseGrepItem の堅牢化 ✅ 対応済（2026-04-25）

`internal/ui/grep_model.go` の `parseGrepItem` を **左から `:数字+:` パターンを走査する実装** に変更。素朴な `SplitN(":", 3)` だと Windows パス `C:\foo\bar.go:10:hit` や ファイル名に `:` を含むケースで誤分解していたが、新実装は「`:` の直後が `\d+`、その次が `:` または文末」という条件を最左マッチで満たす位置を採用するため、`file` 側に `:` を含むケースと `text` 側に `:` を含むケース（`main.go:42:foo:bar:baz`）の両方を正しく扱える。回帰テスト: 既存 3 ケースに加え、Windows パス / `:` 含むファイル名 / text に複数 `:` / text 空 を `TestParseGrepItem` に追加（計 7 ケース）。

#### I-3: 非推奨 API 解消 ✅ 対応済（2026-04-25）

`internal/ui/fuzz_test.go:140` の `tea.MouseLeft`（旧 `MouseEventType`）を新 API `Action: tea.MouseActionPress, Button: tea.MouseButtonLeft` に置き換え。`staticcheck -checks=SA1019` が無指摘になることを確認。`FuzzModelUpdateView` を 5 秒回して新シグネチャでも mouse event の Update が panic なく動くことを確認。

#### I-4: 未使用コード削除 ✅ 対応済（2026-04-25）

`internal/ui/styles.go:143` の `loadingStyle` を削除。`staticcheck` 全チェック無指摘になったことを確認。

---

## Phase 4: Fuzz 強化提案

既存:

- `FuzzParseRgJSON` (`internal/grep`)
- `FuzzModelUpdateView` (`internal/ui`)

追加推奨:

1. ~~**`FuzzSanitizeForTerminal`**（H-1/H-2/H-3 修正後）~~ → ✅ `FuzzForTerminal` (`internal/sanitize/sanitize_test.go`) として実装済。任意バイト列に対し ESC/DEL/C0/BiDi 非含有、UTF-8 valid、冪等を不変条件として検証。

2. ~~**`FuzzParseGrepItem`**~~ → ✅ 2026-04-25 完了 (`internal/ui/model_test.go`)。任意の文字列に対し:
   - panic しない、`line >= 0`、`file` は input の prefix
   - `line > 0` のとき: `input[:len(file)]==file` かつ続く `:` の後の digit run が line と一致し、その後は `:` か文末
   
   **fuzz 検出バグ**: 入力 `":00"` で `parseGrepItem` が `("", 0)` を返していた（line==0 が parse 成功扱いになり、parse 失敗の sentinel と矛盾）。grep 出力の line は 1-based なので `line<=0` を parse 失敗扱いに変更し、`TestParseGrepItem` にも回帰ケース 2 件 追加。15 秒 fuzz で 54万実行、追加発見ゼロ。

3. ~~**`FuzzReadFileRange`**~~ → ✅ 2026-04-25 完了 (`internal/preview/preview_test.go`)。ランダムな content（最大 64KB にキャップ）と任意の `startLine` / `maxLines` で:
   - panic しない
   - 戻り値が valid UTF-8（sanitize 通過の保証）
   - raw ESC / BEL / BiDi 制御が含まれない
   - 戻り値サイズが `len(content)*8 + 1024` 以下（sanitize の visualize 拡張上界）
   
   15 秒 fuzz で発見ゼロ（I/O 律速で約 3.5万実行、30 new interesting でカバレッジ拡大確認）。

---

## 推奨対応順序（ROI 順）

1. ~~**H-1/H-2/H-3**: 共通サニタイザ導入 + 3 経路に適用。1ファイル追加 + 呼出3箇所で完結。~~ → ✅ 2026-04-24 完了（`internal/sanitize` 新設、preview/finder/grep の 3 箇所に適用、`FuzzForTerminal` 含む TDD で実装）。
2. ~~**M-1**: preview の上限導入。~~ → ✅ 2026-04-25 完了（`MaxPreviewSize = 2MB`、`IsTooLarge` + `LargeFileMessage`、境界テスト + 状態メッセージ golden で保護）。
3. ~~**M-2**: エディタ `--` セパレータ追加。~~ → ✅ 2026-04-25 完了（`buildEditorArgs` に抽出し vim/nvim/emacs/vi/code 系/zed を対応、`./` 前置で二層防御、フラグ形状不変条件プロパティテスト付き）。
4. ~~**M-3**: `exec.CommandContext` へ移行。~~ → ✅ 2026-04-25 完了（`grep.Search`/`finder.ListFiles` の ctx 化、`GrepModel.searchCancel` によるデバウンスキャンセル、integration test で実プロセス伝播確認）。
5. ~~**L-2**: `exec.LookPath` で rg/fd を起動時に絶対パス解決。~~ → ✅ 2026-04-25 完了（grep / finder の両方でパッケージレベル var 化、バイナリ未検出時は明示エラー）。
6. ~~**L-3**: rg/fd 子プロセスの env をホワイトリスト化。~~ → ✅ 2026-04-25 完了（`whitelistedEnv()` で `PATH`/`HOME`/`LANG`/`LC_*` のみ通す。エディタは UX 維持のため対象外）。
7. **L-1**（symlink）は ⚠️ **Won't-fix**（2026-04-25 判定）。脅威モデル上、privilege escalation がなく `cat` と同等で glimpse 固有の攻撃面ではないため。詳細は L-1 セクション参照。
8. ~~**Info 系**~~ → ✅ 2026-04-25 完了（I-1〜I-4 全対応。`staticcheck` 無指摘）。
9. ~~**Fuzz 追加**~~ → ✅ 2026-04-25 完了（`FuzzParseGrepItem` / `FuzzReadFileRange` 追加。前者は fuzz 駆動で `:00` 入力の line==0 曖昧バグを 1 件発見し修正）。

---

## 実行コマンド再現手順

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/google/osv-scanner/cmd/osv-scanner@latest

govulncheck ./...
osv-scanner --lockfile go.mod
gosec ./...
staticcheck ./...
go test -race ./...
```

---

## 注意事項

- このレポートは `go-cli-security-audit` skill のチェックリストに基づく **網羅的だが完全ではない**診断。ビジネスロジック特有の脆弱性や、今後追加される機能に紐づく脆弱性は拾えていない。
- 依存関係の CVE は日々更新されるため、`govulncheck` は CI/CD で定期実行することを推奨。
