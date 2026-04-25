package ui

import (
	"fmt"
	"path/filepath"
	"strings"
)

// sanitizeEditorFilePath はファイル名が `-` や `+` で始まる場合に `./` を前置し、
// エディタの引数パーサでフラグと誤認されないよう正規化する。
// 絶対パスや既に `./` で始まるパスはそのまま返す。
func sanitizeEditorFilePath(file string) string {
	if file == "" {
		return file
	}
	if strings.HasPrefix(file, "/") || strings.HasPrefix(file, "./") {
		return file
	}
	if strings.HasPrefix(file, "-") || strings.HasPrefix(file, "+") {
		return "./" + file
	}
	return file
}

// buildEditorArgs はエディタ起動時の引数列を組み立てる。
// エディタ別の行指定・`--` セパレータ配置を吸収し、悪意あるファイル名による
// 引数フラグ注入（例: `-c:!sh`）を防ぐ（M-2 対策）。
//
// 対応エディタ:
//   - vim / nvim / emacs / vi / その他: `+LINE -- FILE`
//   - code / code-insiders / codium / vscodium: `-g FILE[:LINE]`
//     (`-g` が次トークンを値として消費する性質 + `./` 前置で保護)
//   - zed: `-- FILE[:LINE]`
func buildEditorArgs(editor, file string, line int) []string {
	safeFile := sanitizeEditorFilePath(file)
	name := filepath.Base(editor)

	switch name {
	case "code", "code-insiders", "codium", "vscodium":
		target := safeFile
		if line > 0 {
			target = fmt.Sprintf("%s:%d", safeFile, line)
		}
		return []string{"-g", target}
	case "zed":
		target := safeFile
		if line > 0 {
			target = fmt.Sprintf("%s:%d", safeFile, line)
		}
		return []string{"--", target}
	default:
		var args []string
		if line > 0 {
			args = append(args, fmt.Sprintf("+%d", line))
		}
		args = append(args, "--", safeFile)
		return args
	}
}
