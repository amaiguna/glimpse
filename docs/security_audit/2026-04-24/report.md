# セキュリティ監査レポート (2026-04-24)

`go-cli-security-audit` skill のワークフロー（Phase 1〜4）に基づき実施。

- 対象コミット: `cf8e186` (main)
- Go バージョン: `go1.26.2-X:nodwarf5 linux/amd64`
- 対象範囲: プロジェクト全体（`./...`）
- High 項目の再現手順: [`repro/README.md`](repro/README.md)

## サマリ

| 優先度 | 件数 | 概要 |
|---|---|---|
| High | 3 | ターミナルエスケープシーケンス注入（preview/ファイル名/grep 行） |
| Medium | 3 | preview OOM、エディタ引数フラグ注入、`exec.CommandContext` 不使用 |
| Low | 3 | symlink、PATH 汚染、環境変数無制限継承 |
| Info | 4 | stdout サイズ未制限、`parseGrepItem` 境界、非推奨 API、未使用コード |

依存関係・メモリモデルはクリーン（`govulncheck` / `osv-scanner` / `go test -race` すべて無指摘）。
最大リスクは **TUI 特有のターミナルエスケープ注入**で、汎用ツールでは検出できない観点。

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

#### H-1: ファイル内容経由のターミナルエスケープ注入

- 該当箇所: `internal/preview/preview.go:43, 65` → `internal/ui/model.go:334-343` で描画
- 内容: `chroma` は入力テキスト中の `\x1b[…` を除去しない。`ansi.Truncate` は本物の ANSI と区別できないのでそのまま保持する。ファイル内容に制御シーケンスを含ませた悪意あるファイルを preview に表示しただけで、タイトルバー書換・画面クリア+偽プロンプト表示・OSC 52 クリップボード書き込みなどが可能。
- 対策: `os.ReadFile` の結果を chroma に渡す**前**に制御文字サニタイズを適用する。

#### H-2: ファイル名経由のターミナルエスケープ注入

- 該当箇所: `internal/finder/finder.go:8-22`（fd/rg 出力）→ `internal/ui/finder.go:155-182` 描画
- 内容: ファイル名には改行以外のほぼ全バイトを含めることができ、`fd` / `rg --files` はデフォルトでそれらを出力する。悪意ある第三者が `$'\x1b[2J\x1b[H偽プロンプト'` のようなファイルを置いたディレクトリで被害者が glimpse を起動すると、左ペイン描画時に画面が乗っ取られる。
- 対策: View 前に同サニタイザを通す。

#### H-3: grep 行内容経由のターミナルエスケープ注入

- 該当箇所: `internal/grep/grep.go:86`（`rg --json` の `lines.text` を格納）→ `internal/ui/grep_model.go:174-180` 描画
- 内容: `rg --json` の行テキストは生ファイル内容。H-1 と同じ攻撃面が grep モードでも成立。
- 対策: `formatGrepMatches` の時点、あるいは View 直前にサニタイズ。

#### 共通対策: サニタイザ実装例

```go
// internal/ui/sanitize.go（新規）
package ui

import (
    "fmt"
    "strings"
    "unicode"
)

// sanitizeForTerminal は描画経路に流す文字列から
// 改行・タブ以外の制御文字と ANSI エスケープを除去・可視化する。
func sanitizeForTerminal(s string) string {
    var b strings.Builder
    b.Grow(len(s))
    for _, r := range s {
        switch {
        case r == '\n' || r == '\t':
            b.WriteRune(r)
        case r < 0x20 || r == 0x7f:
            b.WriteString(fmt.Sprintf("\\x%02x", r))
        case unicode.IsControl(r):
            b.WriteString(fmt.Sprintf("\\u%04x", r))
        default:
            b.WriteRune(r)
        }
    }
    return b.String()
}
```

Trojan Source (CVE-2021-42574) 対策を強化する場合は `golang.org/x/text/unicode/bidi` で BiDi 制御文字も検出・除去する。

### Medium

#### M-1: preview の無制限メモリ読み込み (OOM)

- 該当箇所: `internal/preview/preview.go:43, 65`
- 内容: `os.ReadFile` はファイル全体を一度にメモリへ読み込む。`ReadFileRange` も全読み込み後に行スライスで切る実装のため、サイズ上限がない。GB 級のログファイルや動画をカーソルで選択しただけで即 OOM。
- 対策: 先頭 Nバイト（例: 1〜4MB）で打ち切る、あるいは `bufio.Scanner` を使った行単位 streaming 読み。サイズ上限は `binarySniffSize` と整合を取った定数で管理するのが望ましい。

#### M-2: エディタ起動時の `--` セパレータ欠如

- 該当箇所: `internal/ui/model.go:286-287`
- 内容: `args = append(args, fmt.Sprintf("+%d", line)); args = append(args, file)` でエディタに引数を渡しているが `--` なし。細工ファイル名 `-c:set shell=...` のようなファイルを Enter で開くと Vim/Neovim が引数扱いして**任意コマンド実行**に繋がる。
- 対策: `args = append(args, "--", file)` を追加。エディタ別に `--` サポート差があるので、`vim`/`nvim`/`emacs`/`code` など主要エディタの互換を確認する。

#### M-3: `exec.CommandContext` 不使用

- 該当箇所: `internal/grep/grep.go:50`, `internal/finder/finder.go:9, 12`
- 内容: `exec.Command` のみで context を渡していない。デバウンス中にキーストローク毎に rg が立ち上がるが、古いプロセスはキャンセルできず stdout がメモリに溜まる。暴走 rg を殺せない。
- 対策: `context.WithTimeout` + `exec.CommandContext`。クエリ変更時に前回 context をキャンセルできる構造にする。

### Low

#### L-1: symlink 経由の任意ファイル参照

- 該当箇所: `internal/preview/preview.go:25, 43, 65`
- 内容: `fd --type f` は regular file のみなので symlink は一覧に出ないが、`rg --files` はデフォルトで symlink を追従しない反面 `--follow` 等で挙動が変わる。ユーザー自身の権限範囲内なので escalation はないが、`~/.ssh/id_rsa` などへのうっかり参照は観点として記録しておく。
- 対策: `os.Lstat` で symlink 検知 → skip、あるいはパスが意図したベースディレクトリ以下か検証（Go 1.24+ なら `os.Root`）。

#### L-2: PATH 汚染対策なし

- 該当箇所: `internal/finder/finder.go:9, 12`, `internal/grep/grep.go:50`
- 内容: `exec.Command("rg", ...)` / `("fd", ...)` で PATH 依存解決。カレント実行ディレクトリ直下に悪意ある `rg`/`fd` があると（Go 1.19+ は `.` を PATH から除外するので影響は小さいが）、PATH 上の順序次第では差し替え可能。
- 対策: 起動時に `exec.LookPath` で絶対パス解決し、以降はそれを使う。任意では解決先が `/usr/bin` 等の想定ディレクトリ下かを検証。

#### L-3: 子プロセスへの環境変数無制限継承

- 該当箇所: 全 `exec.Command` 呼び出し
- 内容: `cmd.Env` 未設定なので親プロセスの全環境変数（`GIT_SSH_COMMAND`, `LD_PRELOAD`, クレデンシャル系）が rg/fd/editor に流れる。
- 対策: 必要なものだけホワイトリスト（`PATH`, `HOME`, `LANG`, `TERM`, `EDITOR` 依存分）。

### Info

| ID | 該当箇所 | 内容 |
|---|---|---|
| I-1 | `internal/finder/finder.go`, `internal/grep/grep.go` | `cmd.Output()` が stdout 全体をメモリ化。巨大リポの `rg --files` は数百MB可能。`StdoutPipe` + `io.LimitReader` が望ましい |
| I-2 | `internal/ui/grep_model.go:297` | `parseGrepItem` は `:` で 2分割。Windows パスや `:` を含むファイル名で壊れる（panic はしない） |
| I-3 | `internal/ui/fuzz_test.go:140` | `tea.MouseLeft` 非推奨 (SA1019) → `MouseAction` + `MouseButton` |
| I-4 | `internal/ui/styles.go:143` | 未使用 `loadingStyle` (U1000) |

---

## Phase 4: Fuzz 強化提案

既存:

- `FuzzParseRgJSON` (`internal/grep`)
- `FuzzModelUpdateView` (`internal/ui`)

追加推奨:

1. **`FuzzSanitizeForTerminal`**（H-1/H-2/H-3 修正後）
   - 任意バイト列で panic せず、出力に制御バイト (0x00–0x1f, 0x7f) や ANSI エスケープ (`\x1b`) が含まれないことを不変条件に。

2. **`FuzzParseGrepItem`** (`internal/ui/grep_model.go:297`)
   - ランダム入力で常に panic しないこと。`strings.SplitN` + `strconv.Atoi` の組み合わせの保険。

3. **`FuzzReadFileRange`** (`internal/preview/preview.go:64`)
   - 一時ファイルにランダム bytes を書き、`startLine`/`maxLines` ランダムで呼んで panic しないこと。M-1 の上限導入後は「戻り値のサイズが上限以下」も不変条件に追加。

---

## 推奨対応順序（ROI 順）

1. **H-1/H-2/H-3**: 共通サニタイザ導入 + 3 経路に適用。1ファイル追加 + 呼出3箇所で完結。
2. **M-1**: preview の上限導入（既存 `binarySniffSize` 周辺に新定数、`ReadFile`/`ReadFileRange` 書き換え）。
3. **M-2**: エディタ `--` セパレータ追加（1行修正）。
4. **M-3**: `exec.CommandContext` へ移行（`grep.Search` / `finder.ListFiles` のシグネチャ拡張が必要）。
5. Low/Info は余裕のあるタイミングで対応。
6. **Fuzz 追加**: サニタイザと ReadFileRange の上限導入後に実施。

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
