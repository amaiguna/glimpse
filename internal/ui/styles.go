package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// カラーパレット
var (
	colorPrimary   = lipgloss.Color("62")  // 青紫
	colorSecondary = lipgloss.Color("241") // グレー
	colorAccent    = lipgloss.Color("170") // ピンク
	colorWhite     = lipgloss.Color("255")
)

// ヘッダー（モードラベル + 入力欄）
var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorWhite).
	PaddingLeft(1)

// モードラベル（アクティブな入力欄に対応するラベル用）
var modeLabelStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorPrimary)

// 非アクティブな入力欄のラベル用（proposal #001 D-3）。
// Shift+Tab でフォーカスが切り替わったとき、active/inactive のラベル色を
// 入れ替えて「いまどちらの入力欄に文字が流れるか」をラベルだけで判別可能にする。
var inactiveLabelStyle = lipgloss.NewStyle().
	Foreground(colorSecondary)

// リストペイン枠
var listPaneStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(colorPrimary)

// プレビューペイン枠
var previewPaneStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(colorSecondary)

// リストアイテム（選択中）
var selectedItemStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorAccent)

// リストアイテム（非選択）
var normalItemStyle = lipgloss.NewStyle().
	Foreground(colorWhite)

// マッチハイライト用の ANSI エスケープ。
// 背景色のみ適用し、シンタックスハイライトの前景色を保持する。
const (
	matchHlStart = "\x1b[48;5;24m\x1b[1m" // 濃い青背景 + 太字
	matchHlEnd   = "\x1b[49m\x1b[22m"     // 背景と太字だけリセット（前景色は維持）
	ansiReset    = "\x1b[0m"
)

// ファジーマッチ用ハイライト ANSI エスケープ (proposal #002 D-1)。
// 当面 matchHlStart/End と同値だが、Finder/Grep 左ペインのファジーマッチ表示と
// grep プレビューのヒット行ハイライトが偶然同じ ANSI を使っているだけで、
// 設計上の依存はない。将来別 style にしたい場合はこの定数の値を変えるだけで済む。
const (
	fuzzyMatchHlStart = matchHlStart
	fuzzyMatchHlEnd   = matchHlEnd
)

// highlightMatches はシンタックスハイライト済みの行から query にマッチする部分を見つけ、
// マッチ箇所にだけ背景ハイライトを適用する。
// シンタックスハイライトの前景色はマッチ部分でも非マッチ部分でも保持される。
func highlightMatches(line, query string) string {
	if query == "" {
		return line
	}
	plain := ansi.Strip(line)
	lowerPlain := strings.ToLower(plain)
	lowerQuery := strings.ToLower(query)

	// プレーンテキスト内のマッチ範囲（バイト位置）を収集
	var ranges [][2]int
	pos := 0
	for {
		idx := strings.Index(lowerPlain[pos:], lowerQuery)
		if idx < 0 {
			break
		}
		start := pos + idx
		end := start + len(lowerQuery)
		ranges = append(ranges, [2]int{start, end})
		pos = end
	}
	if len(ranges) == 0 {
		return line
	}

	// ANSI 文字列を走査し、可視文字のバイト位置を追いながらマッチ境界にハイライトを挿入
	var buf strings.Builder
	visBytePos := 0 // プレーンテキスト上のバイト位置
	rangeIdx := 0
	inMatch := false

	for i := 0; i < len(line); {
		// ANSI エスケープシーケンスをスキップ
		if line[i] == '\x1b' && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && line[j] != 'm' {
				j++
			}
			if j < len(line) {
				j++ // 'm' を含める
				seq := line[i:j]
				buf.WriteString(seq)
				// マッチ中に chroma のリセットが来たらハイライトを再注入
				if inMatch && seq == ansiReset {
					buf.WriteString(matchHlStart)
				}
				i = j
				continue
			}
		}

		// マッチ開始チェック
		if !inMatch && rangeIdx < len(ranges) && visBytePos == ranges[rangeIdx][0] {
			buf.WriteString(matchHlStart)
			inMatch = true
		}

		// 可視文字を出力
		_, size := utf8.DecodeRuneInString(line[i:])
		buf.WriteString(line[i : i+size])
		visBytePos += size
		i += size

		// マッチ終了チェック
		if inMatch && visBytePos == ranges[rangeIdx][1] {
			buf.WriteString(matchHlEnd)
			inMatch = false
			rangeIdx++
		}
	}
	if inMatch {
		buf.WriteString(matchHlEnd)
	}

	return buf.String()
}

// エラー表示
var errorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("196")).
	Bold(true)
