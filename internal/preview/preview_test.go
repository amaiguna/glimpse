package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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

// 悪意あるファイル内容（ANSI エスケープ・OSC・BiDi）が描画安全な形に変換されて返ることを確認。
// H-1 の回帰テスト。
func TestReadFileRangeSanitizesEscapes(t *testing.T) {
	malicious := "Line1\nLine2 \x1b]0;PWNED\x07\nLine3 \x1b[31mRED\x1b[0m\nLine4 \u202eRTL\nLine5\n"
	path := writeTempFile(t, "evil.txt", malicious)

	got, err := ReadFileRange(path, 1, 10)
	require.NoError(t, err)

	assert.NotContains(t, got, "\x1b", "ESC byte must be removed")
	assert.NotContains(t, got, "\x07", "BEL byte must be removed")
	assert.NotContains(t, got, "\u202e", "BiDi RLO must be removed")
	// 通常文字は残る
	assert.Contains(t, got, "Line1")
	assert.Contains(t, got, "RED")
	assert.Contains(t, got, "PWNED")
}

func TestReadFileSanitizesEscapes(t *testing.T) {
	malicious := "before\x1b[2J\x1b[Hafter\n"
	path := writeTempFile(t, "evil.txt", malicious)

	got, err := ReadFile(path, 0)
	require.NoError(t, err)
	assert.NotContains(t, got, "\x1b")
	assert.Contains(t, got, "before")
	assert.Contains(t, got, "after")
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

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{
			name:    "\u901a\u5e38\u306e\u30c6\u30ad\u30b9\u30c8",
			content: []byte("package main\n\nfunc main() {}\n"),
			want:    false,
		},
		{
			name:    "\u7a7a\u30d5\u30a1\u30a4\u30eb",
			content: []byte{},
			want:    false,
		},
		{
			name:    "NUL \u30d0\u30a4\u30c8\u3092\u542b\u3080",
			content: []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00, 'a', 'b'},
			want:    true,
		},
		{
			name:    "\u5148\u982d\u306b NUL",
			content: []byte{0x00, 'a', 'b', 'c'},
			want:    true,
		},
		{
			name:    "UTF-8 \u306e\u65e5\u672c\u8a9e",
			content: []byte("\u3053\u3093\u306b\u3061\u306f\u4e16\u754c\n"),
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.bin")
			require.NoError(t, os.WriteFile(path, tt.content, 0644))
			got, err := IsBinary(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsBinaryNotFound(t *testing.T) {
	_, err := IsBinary("/nonexistent/file.bin")
	assert.Error(t, err)
}

func TestIsTooLarge(t *testing.T) {
	tests := []struct {
		name string
		size int
		want bool
	}{
		{"小さいファイル (1KB)", 1024, false},
		{"上限直前 (MaxPreviewSize-1)", MaxPreviewSize - 1, false},
		{"上限ちょうど (MaxPreviewSize)", MaxPreviewSize, false},
		{"上限超え (MaxPreviewSize+1)", MaxPreviewSize + 1, true},
		{"空ファイル", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.txt")
			require.NoError(t, os.WriteFile(path, make([]byte, tt.size), 0644))
			got, err := IsTooLarge(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsTooLargeNotFound(t *testing.T) {
	_, err := IsTooLarge("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestMaxPreviewSizeIs2MB(t *testing.T) {
	assert.Equal(t, int64(2*1024*1024), int64(MaxPreviewSize),
		"MaxPreviewSize は 2MB であること（変更時はレポート/ドキュメントも更新）")
}

// FuzzReadFileRange はランダムなファイル内容と startLine/maxLines に対する不変条件を検証する。
// プロパティ:
//   - panic しない
//   - 戻り値は valid UTF-8（sanitize 通過後の保証）
//   - 戻り値に raw ESC / BEL / BiDi 制御バイトが含まれない
//   - 戻り値の長さは入力ファイルサイズ + sanitize の visualize 拡張倍率（最大 ~7倍） 以内
func FuzzReadFileRange(f *testing.F) {
	type seed struct {
		content   string
		startLine int
		maxLines  int
	}
	seeds := []seed{
		{"L1\nL2\nL3\n", 1, 0},
		{"", 1, 10},
		{"single line", 1, 1},
		{"\x00\x01\x02\x03", 1, 0},
		{"\xff\xfe\xfd", 1, 0}, // 不正 UTF-8
		{"line\x1b[31mred\x1b[0m", 1, 0},
		{"\u202eRTL\n", 1, 1},
		{strings.Repeat("a\n", 1000), 500, 100},
		{"x", -100, -100},
		{"x", 1<<30, 1 << 30}, // 巨大値
	}
	for _, s := range seeds {
		f.Add(s.content, s.startLine, s.maxLines)
	}

	f.Fuzz(func(t *testing.T, content string, startLine, maxLines int) {
		// 巨大な content による I/O 遅延は fuzz としては不要なので軽くキャップ。
		// （正常系のロジック検証が目的で、巨大ファイル処理は M-1 / IsTooLarge の領分）。
		if len(content) > 64*1024 {
			content = content[:64*1024]
		}
		path := filepath.Join(t.TempDir(), "f.txt")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadFileRange(path, startLine, maxLines)
		if err != nil {
			t.Errorf("unexpected error reading temp file: %v", err)
			return
		}
		if !utf8.ValidString(got) {
			t.Errorf("output is not valid UTF-8: %q", got)
		}
		if strings.ContainsRune(got, 0x1b) {
			t.Errorf("raw ESC present in output: %q", got)
		}
		if strings.ContainsRune(got, 0x07) {
			t.Errorf("raw BEL present in output: %q", got)
		}
		// BiDi 制御（U+202A〜U+202E, U+2066〜U+2069）は sanitize で可視化されるはず。
		bidiRunes := []rune{
			0x202A, 0x202B, 0x202C, 0x202D, 0x202E,
			0x2066, 0x2067, 0x2068, 0x2069,
		}
		for _, r := range bidiRunes {
			if strings.ContainsRune(got, r) {
				t.Errorf("BiDi control U+%04X present in output", r)
			}
		}
		// サイズ上界: sanitize は 1 バイト → 最大 6 文字 ('\\xNN') 程度に膨らむ。
		// 安全側で 8倍 + 余白 を上界にする。
		if int64(len(got)) > int64(len(content))*8+1024 {
			t.Errorf("output size %d exceeds expected upper bound for content size %d",
				len(got), len(content))
		}
	})
}
