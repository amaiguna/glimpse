package finder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// MaxCmdOutputSize は fd / rg --files の stdout を読み込むサイズ上限（I-1）。
// 巨大リポでファイル数が爆発した場合の OOM を防ぐ。1パス~100B として
// 50MB あれば 50万パス前後を保持でき、UI 上限としても十分。
const MaxCmdOutputSize = 50 * 1024 * 1024

// ErrOutputTooLarge は fd / rg の stdout が MaxCmdOutputSize を超えた場合に返るエラー。
var ErrOutputTooLarge = errors.New("finder: command output exceeds size limit")

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

// fdBinary / rgBinary はパッケージ初期化時に LookPath で解決した絶対パス。
// 実行時に相対名で PATH 解決すると、途中での差し替えや別ディレクトリ優先が
// 効く余地が残るため、起動時に一度だけ解決して固定する（L-2）。
// インストールされていない環境では空文字列となり、ListFiles 側で明示エラーを返す。
var (
	fdBinary = lookupBinary("fd")
	rgBinary = lookupBinary("rg")
)

func lookupBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// whitelistedEnv は fd/rg に渡す環境変数を PATH/HOME/LANG/LC_* のみに絞って返す（L-3）。
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

// ListFiles は fd --type f でファイル一覧を取得する。fd が無ければ rg --files にフォールバック。
// ctx のキャンセル/タイムアウトは fd/rg プロセスに伝播する（M-3）。
// ctx 由来のエラーは fd 失敗 → rg 再試行で上書きされないよう優先して返す。
// stdout は MaxCmdOutputSize で打ち切られ、超過時は ErrOutputTooLarge を返す（I-1）。
func ListFiles(ctx context.Context) ([]string, error) {
	var out []byte
	var err error

	if fdBinary != "" {
		cmd := exec.CommandContext(ctx, fdBinary, "--type", "f")
		cmd.Env = whitelistedEnv()
		out, err = runWithLimit(cmd, MaxCmdOutputSize)
	}
	if fdBinary == "" || err != nil {
		// 出力サイズ超過は意図的な失敗なので rg にフォールバックせず即返す。
		if errors.Is(err, ErrOutputTooLarge) {
			return nil, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if rgBinary == "" {
			return nil, errors.New("neither fd nor rg found in $PATH")
		}
		cmd := exec.CommandContext(ctx, rgBinary, "--files")
		cmd.Env = whitelistedEnv()
		out, err = runWithLimit(cmd, MaxCmdOutputSize)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, err
		}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

// MaxCmdStderrSize は子プロセスの stderr を取り込む最大バイト数（#008）。
// fd / rg の単一エラーメッセージは数百〜数 KB なので 64KB あれば十分。
const MaxCmdStderrSize = 64 * 1024

// CmdError は外部コマンドが非0終了したときの構造化エラー（#008）。
// `exit status N` だけの不可解なメッセージで終わらせず、stderr 本文を上位に渡す。
type CmdError struct {
	ExitCode int
	Stderr   string
	Err      error
}

// Error は exit status と stderr 要約を結合した一行を返す。
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

// Unwrap は内側の err を返し errors.Is/As の連鎖を保つ。
func (e *CmdError) Unwrap() error { return e.Err }

// boundedWriter は最初の max バイトだけバッファに蓄え、それ以降は黙って捨てる。
// 子プロセスの stderr が肥大化してもメモリを食い潰さないための保険。
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
