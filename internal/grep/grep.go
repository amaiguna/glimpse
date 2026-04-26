package grep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// MaxCmdOutputSize は rg の stdout を読み込むサイズ上限（I-1）。
// 巨大リポでマッチ数が爆発した場合の OOM を防ぐ。50MB あれば
// 数十万行〜数百万行のマッチを保持でき、UI で扱える上限としても十分。
const MaxCmdOutputSize = 50 * 1024 * 1024

// ErrOutputTooLarge は rg の stdout が MaxCmdOutputSize を超えた場合に返るエラー。
var ErrOutputTooLarge = errors.New("rg: command output exceeds size limit")

// readLimited は r から最大 max バイトまで読み込んで返す。
// max を 1 バイトでも超える入力は ErrOutputTooLarge を返す（黙ってトリミングしない）。
func readLimited(r io.Reader, max int64) ([]byte, error) {
	out, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > max {
		return nil, ErrOutputTooLarge
	}
	return out, nil
}

// MaxCmdStderrSize は子プロセスの stderr を取り込む最大バイト数（#008）。
// rg / fd の単一エラーメッセージは数百〜数 KB なので 64KB あれば十分で、
// それ以降は debug バッファに収まらないので静かにドロップする。
const MaxCmdStderrSize = 64 * 1024

// CmdError は外部コマンドが非0終了したときの構造化エラー（#008）。
// ExitCode と Stderr を呼び出し側に渡し、`exit status N` だけの不可解な
// メッセージで終わらせない。errors.As で取り出して分岐できる。
type CmdError struct {
	ExitCode int
	Stderr   string // 末尾の改行は除去済み
	Err      error  // 通常 *exec.ExitError
}

// Error は exit status と stderr の要約を結合した一行を返す。
func (e *CmdError) Error() string {
	base := "command failed"
	if e.Err != nil {
		base = e.Err.Error()
	}
	stderr := strings.TrimSpace(e.Stderr)
	if stderr == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", base, stderr)
}

// Unwrap は内側の err を返し、errors.Is/As の連鎖を保つ。
func (e *CmdError) Unwrap() error { return e.Err }

// boundedWriter は最初の max バイトだけバッファに書き込み、それ以降は黙ってドロップする。
// stderr 取り込み中に max を超えても Write は成功扱いにし、子プロセスを SIGPIPE で
// 落とさないようにする。
type boundedWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			w.buf.Write(p)
		} else {
			w.buf.Write(p[:remaining])
		}
	}
	return len(p), nil
}

// runWithLimit は cmd を Start → readLimited → Wait の順で実行し、
// stdout が max を超えた時点で ErrOutputTooLarge を返す。
// 超過時は残り stdout を破棄し、子プロセスは ctx キャンセル/SIGPIPE 経由で終了する。
// stderr は MaxCmdStderrSize で打ち切りつつ取り込み、非0終了時は CmdError でラップする（#008）。
func runWithLimit(cmd *exec.Cmd, max int64) ([]byte, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &boundedWriter{buf: &stderrBuf, max: MaxCmdStderrSize}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	out, readErr := readLimited(stdout, max)
	if errors.Is(readErr, ErrOutputTooLarge) {
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return nil, &CmdError{
			ExitCode: exitCode,
			Stderr:   strings.TrimRight(stderrBuf.String(), "\n"),
			Err:      waitErr,
		}
	}
	return out, nil
}

// rgBinary はパッケージ初期化時に LookPath で解決した rg の絶対パス。
// 実行時に相対名 "rg" を PATH 解決すると PATH 汚染（別ディレクトリ優先や
// mid-session での差し替え）の余地が残るため、起動時に一度だけ解決して固定する（L-2）。
// インストールされていない環境では空文字列のままとし、Search 側で明示エラーを返す。
var rgBinary = lookupBinary("rg")

func lookupBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// whitelistedEnv は rg に渡す環境変数を PATH/HOME/LANG/LC_* のみに絞って返す（L-3）。
// LD_PRELOAD・GIT_SSH_COMMAND・クレデンシャル系など危険な変数の継承を防ぐ。
func whitelistedEnv() []string {
	keys := []string{"PATH", "HOME", "LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES"}
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Match はrgの検索結果から抽出した1行分のマッチ情報を表す。
type Match struct {
	File string // マッチしたファイルのパス
	Line int    // マッチした行番号（1始まり）
	Text string // マッチした行の全文（末尾改行なし）
}

// rgMessage は rg --json が出力する1行分のJSONメッセージ。
// Type は "begin"（ファイル検索開始）, "match"（マッチ行）, "end"（ファイル検索終了）, "summary"（全体統計）のいずれか。
// Data の中身は Type によって構造が異なるため json.RawMessage で遅延パースする。
type rgMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// rgMatch は rgMessage の Type が "match" のときの Data の構造。
type rgMatch struct {
	// Path はマッチしたファイルのパス情報。
	Path struct {
		Text string `json:"text"`
	} `json:"path"`
	// Lines はマッチした行の全文。末尾に改行を含む。
	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`
	// LineNumber はファイル内の行番号（1始まり）。
	LineNumber int `json:"line_number"`
	// Submatches は行内でパターンにマッチした部分のリスト。
	// Start/End は行内のバイト位置で、ハイライト表示に使える。
	Submatches []struct {
		Match struct {
			Text string `json:"text"` // マッチした文字列
		} `json:"match"`
		Start int `json:"start"` // 行内の開始バイト位置
		End   int `json:"end"`   // 行内の終了バイト位置
	} `json:"submatches"`
}

// Search は rg --json を実行し、結果をパースして返す。
// マッチなし（終了コード1）の場合は nil, nil を返す。
// ctx のキャンセル/タイムアウトは rg プロセスに伝播し、呼び出し側は
// 古いデバウンスをキャンセルして stdout の溜め込みを防げる。
// stdout は MaxCmdOutputSize で打ち切られ、超過時は ErrOutputTooLarge を返す（I-1）。
func Search(ctx context.Context, pattern string) ([]Match, error) {
	if rgBinary == "" {
		return nil, errors.New("rg: executable not found in $PATH")
	}
	cmd := exec.CommandContext(ctx, rgBinary, "--json", pattern)
	cmd.Env = whitelistedEnv()

	out, err := runWithLimit(cmd, MaxCmdOutputSize)
	if err != nil {
		if errors.Is(err, ErrOutputTooLarge) {
			return nil, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		var cmdErr *CmdError
		if errors.As(err, &cmdErr) && cmdErr.ExitCode == 1 {
			return nil, nil
		}
		return nil, err
	}
	return ParseRgJSON(string(out))
}

// ParseRgJSON は rg --json の出力文字列をパースし、マッチ行のみを抽出する。
// 不正なJSONやmatch以外のメッセージ（begin, end, summary）はスキップする。
func ParseRgJSON(output string) ([]Match, error) {
	var matches []Match
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		var msg rgMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type != "match" {
			continue
		}
		var data rgMatch
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			continue
		}
		if data.Path.Text == "" {
			continue
		}
		matches = append(matches, Match{
			File: data.Path.Text,
			Line: data.LineNumber,
			Text: strings.TrimRight(data.Lines.Text, "\n"),
		})
	}
	return matches, nil
}
