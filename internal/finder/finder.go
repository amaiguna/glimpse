package finder

import (
	"context"
	"errors"
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

// runWithLimit は cmd を Start → readLimited → Wait の順で実行し、
// stdout が max を超えた時点で ErrOutputTooLarge を返す。
// 超過時は残り stdout を破棄し、子プロセスは ctx キャンセル/SIGPIPE 経由で終了する。
func runWithLimit(cmd *exec.Cmd, max int64) ([]byte, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
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
	return out, waitErr
}
