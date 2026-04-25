// Package sanitize は外部由来の文字列をターミナル描画に流す前に無害化するユーティリティを提供する。
//
// fuzzy finder は信頼境界を越えた文字列（ファイル内容・ファイル名・grep 結果）を
// そのまま `View()` に乗せて描画するため、入力中に ANSI エスケープシーケンスや
// BiDi 制御文字が含まれていると、タイトル書換・画面クリア・偽プロンプト表示・
// Trojan Source 攻撃などが成立する。
//
// 本パッケージはそれらを可視化された安全な表現に置換する。
package sanitize

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ForTerminal は文字列を「ターミナルに直接描画しても副作用が出ない」表現に変換する。
//
// 変換規則:
//   - 改行 (LF) とタブはそのまま保持する。
//   - C0 制御文字 (< 0x20、ただし LF/TAB を除く) と DEL (0x7f) は \xNN に置換する。
//   - C1 制御文字 (Cc) と BiDi 等のフォーマット文字 (Cf) は \uNNNN に置換する。
//   - 不正な UTF-8 バイトは \xNN として出力する。
//   - 印字可能 ASCII および通常の Unicode 文字 (日本語・絵文字等) は素通し。
//
// 出力は常に valid UTF-8、かつ冪等 (ForTerminal(ForTerminal(s)) == ForTerminal(s))。
func ForTerminal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, `\x%02x`, s[i])
			i++
			continue
		}
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case unicode.In(r, unicode.Cc, unicode.Cf):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}
