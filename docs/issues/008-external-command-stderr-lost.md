# Issue #008: 外部コマンドの stderr 喪失でエラー診断が無効化

**Status:** Closed (2026-04-26) — `internal/grep` / `internal/finder` の双方に `CmdError{ExitCode, Stderr, Err}` と `MaxCmdStderrSize`(64KB) を導入。`runWithLimit` が `cmd.Stderr` を `boundedWriter` に取り込み、非0終了時に CmdError でラップして上位に伝搬する。`grep.Search` の exit-1-no-match 判定は `errors.As` 経由に切替。

## 現状

`internal/grep/grep.go` の `runWithLimit`、`internal/finder/finder.go` の `runWithLimit` ともに `cmd.StdoutPipe` のみ設定し、`cmd.Stderr` を未設定のまま実行している。

```go
func runWithLimit(cmd *exec.Cmd, max int64) ([]byte, error) {
    stdout, err := cmd.StdoutPipe()
    if err != nil { return nil, err }
    if err := cmd.Start(); err != nil { return nil, err }
    out, readErr := readLimited(stdout, max)
    ...
    waitErr := cmd.Wait()
    ...
}
```

Go の `exec.Cmd` は `Stderr == nil` のとき子の stderr を `/dev/null` 相当に接続する。よって rg / fd の診断メッセージはすべて捨てられる。

### 観測される具体例

| コマンド | stderr | exit | 上位に伝わる |
|---|---|---|---|
| `rg --json '[unclosed'` | `regex parse error: unclosed character class` | 2 | `*exec.ExitError("exit status 2")` のみ |
| `fd --type=invalidtype` | `error: invalid value 'invalidtype' for '--type'` | 2 | `*exec.ExitError("exit status 2")` のみ |
| `rg --json '...'` の I/O エラー（permission, not-a-dir 等） | rg が stderr に出す | 1〜2 | exit code すら混同 |

### 影響

- ユーザーへのエラー表示が `exit status N` という UNIX 識別子だけになり意味不明
- exit code 2 が「regex parse error」「invalid argument」「permission denied」のどれかをコード側で**区別できない**
- バグ調査時に「rg が何を訴えていたか」がログにも残らない

## 対応方針の選択肢

### A. cmd.Stderr に bytes.Buffer を割り当てて捕捉

最小変更。`runWithLimit` に `var stderrBuf bytes.Buffer; cmd.Stderr = &stderrBuf` を追加し、エラー時にそのバイト列を文字列としてエラーメッセージに含める。

- stderr もサイズ上限が必要（rg の warning 等で stderr が肥大する可能性。stdout 側の `MaxCmdOutputSize` と同等の安全網が要る）。実装としては `io.MultiWriter` + 制限付き writer、または書き込み時に閾値チェックする `limitedWriter`。
- 取得した stderr メッセージは新たな構造化エラー型（例: `*CmdError{ExitCode int; Stderr string; Err error}`）にラップして返す。

### B. cmd.Output() スタイルに戻す

`exec.Cmd.Output()` は `Stderr == nil` のとき自動で `*exec.ExitError.Stderr` に格納する。ただし I-1 の対応で stdout を `LimitReader` 化したばかりであり、`Output()` に戻すと stdout 側のメモリ上限が消えてしまうため不適。

### C. stdout / stderr 両方にサイズ制限付き pipe を立てる

最も robust。実装コストは A より高いが、stderr のサイズ上限を確実に強制できる。

## 判断

**A を採用**。stderr のサイズ上限は 64KB で実用上十分（rg / fd の単一エラーメッセージは数百バイト〜数KB）。構造化エラー型を導入し、上位レイヤ（`grep.Search`、`finder.ListFiles`、UI）は exit code と stderr 文字列で挙動分岐できるようにする。

実装の指針:

```go
type CmdError struct {
    ExitCode int
    Stderr   string
    Err      error // *exec.ExitError か wrapper
}
func (e *CmdError) Error() string { ... }
func (e *CmdError) Unwrap() error { return e.Err }
```

これにより `errors.As(err, &cmdErr)` で構造化情報を取り出せる。

## 関連 issue

- #007 grep regex 入力時 UI 崩壊（本 issue の症状の 1 つ）
- #010 エディタ起動失敗の黙殺（同種の問題、ただし `tea.ExecProcess` 経由で stderr の扱いが違う）
- セキュリティ監査 I-1（stdout サイズ上限化）と整合

## 優先度

**高** — 単独でデバッグ性を大きく改善し、#007 / #009 / #010 のいずれの修正にも前提として効く。
