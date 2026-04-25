package finder

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

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
func ListFiles(ctx context.Context) ([]string, error) {
	var out []byte
	var err error

	if fdBinary != "" {
		cmd := exec.CommandContext(ctx, fdBinary, "--type", "f")
		cmd.Env = whitelistedEnv()
		out, err = cmd.Output()
	}
	if fdBinary == "" || err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if rgBinary == "" {
			return nil, errors.New("neither fd nor rg found in $PATH")
		}
		cmd := exec.CommandContext(ctx, rgBinary, "--files")
		cmd.Env = whitelistedEnv()
		out, err = cmd.Output()
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
