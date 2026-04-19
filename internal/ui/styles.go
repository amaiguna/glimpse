package ui

import "github.com/charmbracelet/lipgloss"

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

// モードラベル
var modeLabelStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorPrimary)

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

// プレビュー該当行ハイライト
var highlightLineStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("24")).  // 濃い青背景（青黒ターミナルで映える）
	Foreground(lipgloss.Color("228")). // 明るい黄色文字
	Bold(true)

// エラー表示
var errorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("196")).
	Bold(true)

// ローディング表示
var loadingStyle = lipgloss.NewStyle().
	Foreground(colorSecondary).
	Italic(true)
