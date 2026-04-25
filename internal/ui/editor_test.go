package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// M-2 回帰: エディタ起動コマンドの引数組み立て。
// - vim/nvim/emacs 系: `+LINE -- FILE` 形式で `--` セパレータを付ける
// - code 系（code / code-insiders / codium / vscodium）: `-g FILE:LINE` 形式
// - zed: `-- FILE:LINE` 形式
// - 不明なエディタ: vim 系にフォールバック
// さらに、`-` `+` で始まるファイル名は `./` プレフィックスを付加して
// フラグと lexically に区別する。
func TestBuildEditorArgs(t *testing.T) {
	tests := []struct {
		name   string
		editor string
		file   string
		line   int
		want   []string
	}{
		// vim 系
		{"vim 行指定なし", "vim", "file.go", 0, []string{"--", "file.go"}},
		{"vim 行指定あり", "vim", "file.go", 10, []string{"+10", "--", "file.go"}},
		{"nvim 行指定あり", "nvim", "file.go", 5, []string{"+5", "--", "file.go"}},
		{"emacs 行指定あり", "emacs", "file.go", 5, []string{"+5", "--", "file.go"}},
		{"vi 行指定あり", "vi", "file.go", 1, []string{"+1", "--", "file.go"}},
		{"絶対パスエディタは basename で判定", "/usr/local/bin/nvim", "file.go", 10,
			[]string{"+10", "--", "file.go"}},
		{"未知エディタは vim 系にフォールバック", "helix", "file.go", 10,
			[]string{"+10", "--", "file.go"}},

		// VS Code 系
		{"code 行指定なし", "code", "file.go", 0, []string{"-g", "file.go"}},
		{"code 行指定あり", "code", "file.go", 10, []string{"-g", "file.go:10"}},
		{"code-insiders", "code-insiders", "file.go", 5, []string{"-g", "file.go:5"}},
		{"codium", "codium", "file.go", 5, []string{"-g", "file.go:5"}},
		{"vscodium", "vscodium", "file.go", 5, []string{"-g", "file.go:5"}},

		// zed
		{"zed 行指定なし", "zed", "file.go", 0, []string{"--", "file.go"}},
		{"zed 行指定あり", "zed", "file.go", 10, []string{"--", "file.go:10"}},

		// M-2 本命: `-`/`+` 始まりの悪意あるファイル名
		{"vim: -evil.go は ./-evil.go に正規化", "vim", "-evil.go", 10,
			[]string{"+10", "--", "./-evil.go"}},
		{"vim: +evil.go は ./+evil.go に正規化", "vim", "+evil.go", 10,
			[]string{"+10", "--", "./+evil.go"}},
		{"code: -evil.go は ./-evil.go に正規化", "code", "-evil.go", 10,
			[]string{"-g", "./-evil.go:10"}},
		{"zed: -evil.go は ./-evil.go に正規化", "zed", "-evil.go", 10,
			[]string{"--", "./-evil.go:10"}},

		// 正常パスはそのまま
		{"絶対パスはそのまま", "vim", "/abs/path.go", 10,
			[]string{"+10", "--", "/abs/path.go"}},
		{"既に ./ で始まるパスはそのまま", "vim", "./foo.go", 10,
			[]string{"+10", "--", "./foo.go"}},
		{"通常の相対パスはそのまま", "vim", "internal/ui/model.go", 0,
			[]string{"--", "internal/ui/model.go"}},

		// 境界: line <= 0 は行指定なし扱い
		{"line=-1 は 0 と同じ扱い", "vim", "file.go", -1, []string{"--", "file.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEditorArgs(tt.editor, tt.file, tt.line)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 「ファイル名がフラグに化ける」不変条件: 生成された argv には
// ユーザー由来のトークンが `-` `+` で始まるものは含まれない（`./` で正規化される）。
// ただし実装側で作る `+LINE` `-g` `--` は除外する。
func TestBuildEditorArgsNoFlagShaped(t *testing.T) {
	allowed := map[string]bool{"--": true, "-g": true}
	editors := []string{"vim", "nvim", "emacs", "code", "zed", "helix"}
	malicious := []string{"-rf", "-c:!sh", "+norc", "-S/etc/passwd"}

	for _, ed := range editors {
		for _, file := range malicious {
			args := buildEditorArgs(ed, file, 7)
			for _, a := range args {
				if allowed[a] {
					continue
				}
				// 実装が生成する `+LINE` も許容
				if len(a) > 1 && a[0] == '+' && a[1] >= '0' && a[1] <= '9' {
					continue
				}
				assert.False(t, a != "" && (a[0] == '-' || a[0] == '+'),
					"editor=%s file=%q: ユーザー由来トークン %q がフラグ形状で残っている", ed, file, a)
			}
		}
	}
}
