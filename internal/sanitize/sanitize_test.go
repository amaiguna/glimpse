package sanitize

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestForTerminal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"空文字列", "", ""},
		{"印字可能ASCIIはそのまま", "Hello, world! 123", "Hello, world! 123"},
		{"タブと改行は保持", "a\tb\nc", "a\tb\nc"},
		{"ESC (0x1b) は \\x1b 化", "\x1b", `\x1b`},
		{"OSC タイトル書換", "\x1b]0;PWNED\x07", `\x1b]0;PWNED\x07`},
		{"SGR カラー", "\x1b[31mRED\x1b[0m", `\x1b[31mRED\x1b[0m`},
		{"画面クリア", "\x1b[2J\x1b[H", `\x1b[2J\x1b[H`},
		{"DEL (0x7f)", "\x7f", `\x7f`},
		{"BEL (0x07)", "\x07", `\x07`},
		{"NUL (0x00)", "\x00", `\x00`},
		{"CR (0x0d) も制御扱い", "\r", `\x0d`},
		{"日本語 UTF-8 は素通し", "こんにちは世界", "こんにちは世界"},
		{"絵文字も素通し", "🌟✨", "🌟✨"},
		{"BiDi RLO (U+202E) は Trojan Source 対策で除去", "\u202e", `\u202e`},
		{"BiDi LRO (U+202D)", "\u202d", `\u202d`},
		{"BiDi RLI (U+2067)", "\u2067", `\u2067`},
		{"C1 制御 (U+0080)", "\u0080", `\u0080`},
		{"C1 制御 (U+009f)", "\u009f", `\u009f`},
		{"不正 UTF-8 バイトは \\xNN 化", "abc\xff\xfedef", `abc\xff\xfedef`},
		{"複合: 通常テキスト + ESC + 通常", "before\x1b[31mafter", `before\x1b[31mafter`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ForTerminal(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 冪等性: sanitize(sanitize(x)) == sanitize(x) を保証する。
// 二重適用でエスケープが壊れないことの保険。
func TestForTerminalIdempotent(t *testing.T) {
	inputs := []string{
		"",
		"plain text",
		"\x1b[31mRED\x1b[0m",
		"\x1b]0;TITLE\x07",
		"tab\there\nnewline",
		"\u202eRTL",
		"\xff bad utf8",
		"日本語",
	}
	for _, in := range inputs {
		once := ForTerminal(in)
		twice := ForTerminal(once)
		assert.Equal(t, once, twice, "input=%q", in)
	}
}

// 不変条件 fuzz:
//
//   - 出力に ESC バイト (0x1b) を含まない
//   - 出力に DEL (0x7f) を含まない
//   - 出力に \n, \t 以外の C0 制御文字を含まない
//   - 出力に BiDi 制御文字 (Trojan Source) を含まない
//   - 出力は常に valid UTF-8
//   - 冪等
func FuzzForTerminal(f *testing.F) {
	seeds := []string{
		"",
		"abc",
		"\x1b[31m",
		"\x1b]0;X\x07",
		"\x00\x01\x02",
		"\u202eEvil",
		"日本語\x1b[2J",
		"\xff\xfe",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		out := ForTerminal(input)

		assert.False(t, strings.ContainsRune(out, 0x1b), "ESC found: %q", out)
		assert.False(t, strings.ContainsRune(out, 0x7f), "DEL found: %q", out)

		for _, r := range out {
			if r < 0x20 && r != '\n' && r != '\t' {
				t.Fatalf("C0 control %U found: %q", r, out)
			}
			switch r {
			case '\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
				'\u2066', '\u2067', '\u2068', '\u2069':
				t.Fatalf("BiDi control %U found: %q", r, out)
			}
		}

		assert.True(t, utf8.ValidString(out), "non-UTF8 output: %q", out)
		assert.Equal(t, out, ForTerminal(out), "idempotence violated: %q", input)
	})
}
