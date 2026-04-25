package finder

import (
	"context"
	"os/exec"
	"strings"
)

// ListFiles は fd --type f でファイル一覧を取得する。fd が無ければ rg --files にフォールバック。
// ctx のキャンセル/タイムアウトは fd/rg プロセスに伝播する（M-3）。
// ctx 由来のエラーは fd 失敗 → rg 再試行で上書きされないよう優先して返す。
func ListFiles(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "fd", "--type", "f")
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		cmd = exec.CommandContext(ctx, "rg", "--files")
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
