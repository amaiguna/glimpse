package preview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/amaiguna/glimpse-tui/internal/sanitize"
)

// binarySniffSize はバイナリ判定のために先頭から読み込むバイト数。
const binarySniffSize = 8192

// BinaryFileMessage はバイナリファイルが選択された際にプレビュー欄に表示する文字列。
const BinaryFileMessage = "バイナリファイルはプレビューできません"

// IsBinary はファイルがバイナリかどうかを判定する。
// 先頭 binarySniffSize バイトに NUL バイト (0x00) が含まれていれば true を返す
// （Git と同じ方式）。
func IsBinary(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, binarySniffSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		// EOF（空ファイル）はバイナリではない
		return false, nil
	}
	return bytes.IndexByte(buf[:n], 0x00) >= 0, nil
}

// ReadFile はファイルを読み込み、最大 maxLines 行まで返す。
// maxLines が 0 以下の場合は全行を返す。
// 戻り値は描画前提として sanitize.ForTerminal で無害化済み。
func ReadFile(path string, maxLines int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if content == "" {
		return "", nil
	}
	if maxLines > 0 {
		lines := strings.SplitN(content, "\n", maxLines+1)
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		content = strings.Join(lines, "\n") + "\n"
	}
	return sanitize.ForTerminal(content), nil
}

// ReadFileRange はファイルの startLine 行目（1-based）から最大 maxLines 行を読み込んで返す。
// startLine が 1 未満の場合は 1 として扱う。
// 戻り値は描画前提として sanitize.ForTerminal で無害化済み。
func ReadFileRange(path string, startLine, maxLines int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if content == "" {
		return "", nil
	}
	if startLine < 1 {
		startLine = 1
	}

	lines := strings.Split(content, "\n")
	// startLine は 1-based なのでスライスは startLine-1 から
	if startLine-1 >= len(lines) {
		return "", nil
	}
	lines = lines[startLine-1:]
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return sanitize.ForTerminal(strings.Join(lines, "\n")), nil
}

// Highlight はファイルパスから言語を推定し、シンタックスハイライト付きの ANSI 文字列を返す。
// 空文字列の場合はそのまま空文字列を返す。
func Highlight(path string, content string) (string, error) {
	if content == "" {
		return "", nil
	}

	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("monokai")
	formatter := formatters.Get("terminal256")

	tokens, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content, nil
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, tokens); err != nil {
		return content, nil
	}

	return buf.String(), nil
}
