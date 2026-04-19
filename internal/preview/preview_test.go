package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// テスト用の一時ファイルを作成するヘルパー。
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestReadFileMaxLines(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"

	tests := []struct {
		name      string
		maxLines  int
		wantLines int
	}{
		{
			name:      "全行読み込み（maxLines=0）",
			maxLines:  0,
			wantLines: 5,
		},
		{
			name:      "先頭3行のみ",
			maxLines:  3,
			wantLines: 3,
		},
		{
			name:      "maxLinesがファイル行数を超える場合は全行返す",
			maxLines:  100,
			wantLines: 5,
		},
		{
			name:      "1行のみ",
			maxLines:  1,
			wantLines: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, "test.txt", content)
			got, err := ReadFile(path, tt.maxLines)
			require.NoError(t, err)

			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
			assert.Equal(t, tt.wantLines, len(lines))
		})
	}
}

func TestReadFileNotFound(t *testing.T) {
	_, err := ReadFile("/nonexistent/file.txt", 0)
	assert.Error(t, err)
}

func TestReadFileEmpty(t *testing.T) {
	path := writeTempFile(t, "empty.txt", "")
	got, err := ReadFile(path, 0)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestReadFileRange(t *testing.T) {
	// 10行のファイル
	content := "L1\nL2\nL3\nL4\nL5\nL6\nL7\nL8\nL9\nL10\n"

	tests := []struct {
		name      string
		startLine int
		maxLines  int
		want      string
	}{
		{"先頭から3行", 1, 3, "L1\nL2\nL3"},
		{"3行目から3行", 3, 3, "L3\nL4\nL5"},
		{"末尾付近", 9, 5, "L9\nL10\n"},
		{"startLine が 0 の場合は 1 として扱う", 0, 2, "L1\nL2"},
		{"startLine が負の場合は 1 として扱う", -5, 2, "L1\nL2"},
		{"範囲外の startLine", 100, 3, ""},
		{"maxLines が 0 なら全行", 3, 0, "L3\nL4\nL5\nL6\nL7\nL8\nL9\nL10\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, "range.txt", content)
			got, err := ReadFileRange(path, tt.startLine, tt.maxLines)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadFileRangeNotFound(t *testing.T) {
	_, err := ReadFileRange("/nonexistent/file.txt", 1, 10)
	assert.Error(t, err)
}

func TestHighlightDetectsLanguage(t *testing.T) {
	goCode := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	path := writeTempFile(t, "main.go", goCode)

	got, err := Highlight(path, goCode)
	require.NoError(t, err)
	// ハイライトされた出力には ANSI エスケープシーケンスが含まれる
	assert.Contains(t, got, "\x1b[", "ANSI エスケープが含まれる")
	// 元のコード内容も含まれる
	assert.Contains(t, got, "main")
}

func TestHighlightUnknownExtension(t *testing.T) {
	content := "just plain text\n"
	path := writeTempFile(t, "file.xyz123", content)

	got, err := Highlight(path, content)
	require.NoError(t, err)
	// 未知の拡張子でもエラーにはならず、テキストがそのまま返される
	assert.Contains(t, got, "just plain text")
}

func TestHighlightEmptyContent(t *testing.T) {
	path := writeTempFile(t, "empty.go", "")
	got, err := Highlight(path, "")
	require.NoError(t, err)
	assert.Equal(t, "", got)
}
