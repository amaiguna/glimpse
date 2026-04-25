package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amaiguna/glimpse-tui/internal/grep"
	"github.com/amaiguna/glimpse-tui/internal/sanitize"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// debounceInterval は Grep モードでの入力デバウンス間隔。
const debounceInterval = 300 * time.Millisecond

// GrepModel はライブ grep モードのペイン。
// rg --json をデバウンス付きで実行し、結果を表示する。
type GrepModel struct {
	textInput        textinput.Model
	items            []string // "file:line:text" 形式
	cursor           int
	offset           int // スクロールオフセット（表示先頭行）
	viewHeight       int // 表示可能行数（親から設定）
	viewWidth        int // 表示可能幅（親から設定）
	loading          bool
	err              error
	debounceTag      int
	previewStartLine int // PreviewRange が最後に返した開始行（DecoratePreview で使用）
}

// NewGrepModel は GrepModel を初期化して返す。
func NewGrepModel() *GrepModel {
	ti := textinput.New()
	ti.Placeholder = "Search pattern..."
	ti.Focus()
	ti.CharLimit = 256
	return &GrepModel{
		textInput: ti,
	}
}

// runGrepCmd は rg 検索を非同期で実行するコマンドを返す。
func runGrepCmd(pattern string) tea.Cmd {
	return func() tea.Msg {
		matches, err := grep.Search(pattern)
		if err != nil {
			return GrepErrorMsg{Err: err}
		}
		return GrepDoneMsg{Matches: matches}
	}
}

func (g *GrepModel) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return g.handleKey(msg)
	case GrepDoneMsg:
		g.loading = false
		g.err = nil
		g.items = formatGrepMatches(msg.Matches)
		g.cursor = 0
	case GrepErrorMsg:
		g.loading = false
		g.err = msg.Err
	case debounceTickMsg:
		return g.handleDebounceTick(msg)
	}
	return g, nil
}

func (g *GrepModel) handleKey(msg tea.KeyMsg) (Pane, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if g.cursor > 0 {
			g.cursor--
			g.clampOffset()
		}
		return g, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if len(g.items) > 0 && g.cursor < len(g.items)-1 {
			g.cursor++
			g.clampOffset()
		}
		return g, nil
	default:
		// テキスト入力は textinput に委譲
		prevQuery := g.textInput.Value()
		var cmd tea.Cmd
		g.textInput, cmd = g.textInput.Update(msg)
		if g.textInput.Value() != prevQuery {
			g.debounceTag++
			tag := g.debounceTag
			debounceCmd := tea.Tick(debounceInterval, func(time.Time) tea.Msg {
				return debounceTickMsg{tag: tag}
			})
			return g, tea.Batch(cmd, debounceCmd)
		}
		return g, cmd
	}
}

func (g *GrepModel) handleDebounceTick(msg debounceTickMsg) (Pane, tea.Cmd) {
	if msg.tag != g.debounceTag {
		return g, nil
	}
	if g.textInput.Value() == "" {
		g.items = nil
		return g, nil
	}
	g.loading = true
	return g, runGrepCmd(g.textInput.Value())
}

// clampOffset はカーソルが表示範囲内に収まるよう offset を調整する。
func (g *GrepModel) clampOffset() {
	// cursor を items 範囲内にクランプ
	if g.cursor < 0 {
		g.cursor = 0
	}
	if len(g.items) > 0 && g.cursor >= len(g.items) {
		g.cursor = len(g.items) - 1
	}
	h := g.visibleHeight()
	if h <= 0 {
		return
	}
	if g.cursor < g.offset {
		g.offset = g.cursor
	}
	if g.cursor >= g.offset+h {
		g.offset = g.cursor - h + 1
	}
}

// visibleHeight は表示可能行数を返す。
func (g *GrepModel) visibleHeight() int {
	if g.viewHeight > 0 {
		return g.viewHeight
	}
	return len(g.items)
}

// SetViewSize は親から表示可能な行数と幅を設定する。
func (g *GrepModel) SetViewSize(h, w int) {
	g.viewHeight = h
	g.viewWidth = w
	g.clampOffset()
}

func (g *GrepModel) View() string {
	h := g.visibleHeight()
	if g.offset < 0 || g.offset > len(g.items) {
		g.offset = 0
	}
	end := g.offset + h
	if end > len(g.items) {
		end = len(g.items)
	}
	visible := g.items[g.offset:end]

	// カーソル記号 "> " の分を引いた残り幅
	itemWidth := g.viewWidth - 2

	var b strings.Builder
	for i, item := range visible {
		if i > 0 {
			b.WriteString("\n")
		}
		absIdx := g.offset + i
		displayItem, _ := parseGrepItem(item)
		// 描画用にサニタイズ。SelectedItem/FilePath/OpenTarget は raw のまま使うため
		// ここで保持される items 自体は変更しない。
		display := truncateToWidth(sanitize.ForTerminal(displayItem), itemWidth)
		if absIdx == g.cursor {
			b.WriteString(selectedItemStyle.Render("> " + display))
		} else {
			b.WriteString(normalItemStyle.Render("  " + display))
		}
	}
	return b.String()
}

func (g *GrepModel) SelectedItem() string {
	if len(g.items) == 0 {
		return ""
	}
	return g.items[g.cursor]
}

// FilePath はプレビュー用のファイルパスを返す。"file:line:text" からファイルパスを抽出する。
func (g *GrepModel) FilePath() string {
	item := g.SelectedItem()
	if item == "" {
		return ""
	}
	path, _ := parseGrepItem(item)
	return path
}

func (g *GrepModel) Query() string   { return g.textInput.Value() }
func (g *GrepModel) IsLoading() bool { return g.loading }
func (g *GrepModel) Err() error      { return g.err }

// PreviewRange はプレビューの表示開始行（1-based）を返す。
// ヒット行がプレビューの中央付近に来るよう計算する。
func (g *GrepModel) PreviewRange(visibleHeight int) int {
	item := g.SelectedItem()
	if item == "" {
		g.previewStartLine = 1
		return 1
	}
	_, hitLine := parseGrepItem(item)
	if hitLine <= 0 {
		g.previewStartLine = 1
		return 1
	}
	start := hitLine - visibleHeight/2
	if start < 1 {
		start = 1
	}
	g.previewStartLine = start
	return start
}

// DecoratePreview はプレビューコンテンツの該当行にハイライトを適用する。
// 行番号は PreviewRange で返した開始行からの相対位置で計算する。
func (g *GrepModel) DecoratePreview(content string, width int) string {
	if content == "" {
		return content
	}
	item := g.SelectedItem()
	if item == "" {
		return content
	}
	_, lineNum := parseGrepItem(item)
	if lineNum <= 0 {
		return content
	}

	// previewStartLine からの相対インデックスに変換
	relIdx := lineNum - g.previewStartLine
	lines := strings.Split(content, "\n")
	if relIdx >= 0 && relIdx < len(lines) {
		lines[relIdx] = highlightMatches(lines[relIdx], g.Query())
	}
	return strings.Join(lines, "\n")
}

// TextInput は入力欄のモデルを返す。
func (g *GrepModel) TextInput() textinput.Model { return g.textInput }

// TextInputView はヘッダー用テキスト入力の View 文字列を返す。
func (g *GrepModel) TextInputView() string { return g.textInput.View() }

// OpenTarget はエディタで開く対象を返す。Grep は "file:line:text" からファイルと行番号を抽出する。
func (g *GrepModel) OpenTarget() (string, int) {
	selected := g.SelectedItem()
	if selected == "" {
		return "", 0
	}
	return parseGrepItem(selected)
}

// Reset はモード切替時にペインの状態をリセットする。
func (g *GrepModel) Reset() {
	g.textInput.SetValue("")
	g.cursor = 0
	g.offset = 0
	g.items = nil
	g.err = nil
}

// Focus はテキスト入力にフォーカスを当てる。
func (g *GrepModel) Focus() tea.Cmd {
	return g.textInput.Focus()
}

// Blur はテキスト入力のフォーカスを外す。
func (g *GrepModel) Blur() {
	g.textInput.Blur()
}

// --- ヘルパー関数 ---

// formatGrepMatches は grep.Match のスライスを "file:line:text" 形式の文字列スライスに変換する。
func formatGrepMatches(matches []grep.Match) []string {
	if matches == nil {
		return nil
	}
	items := make([]string, len(matches))
	for i, m := range matches {
		items[i] = fmt.Sprintf("%s:%d:%s", m.File, m.Line, m.Text)
	}
	return items
}

// parseGrepItem は "file:line:text" 形式の文字列からファイルパスと行番号を抽出する。
func parseGrepItem(item string) (string, int) {
	parts := strings.SplitN(item, ":", 3)
	if len(parts) < 2 {
		return item, 0
	}
	line, err := strconv.Atoi(parts[1])
	if err != nil {
		return item, 0
	}
	return parts[0], line
}
