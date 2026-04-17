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
)

// ReadFile はファイルを読み込み、最大 maxLines 行まで返す。
// maxLines が 0 以下の場合は全行を返す。
func ReadFile(path string, maxLines int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if content == "" {
		return "", nil
	}
	if maxLines <= 0 {
		return content, nil
	}

	lines := strings.SplitN(content, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n") + "\n", nil
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
