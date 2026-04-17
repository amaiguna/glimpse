package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amaiguna/telescope-tui/internal/grep"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// debounceInterval は Grep モードでの入力デバウンス間隔。
const debounceInterval = 300 * time.Millisecond

// GrepModel はライブ grep モードのペイン。
// rg --json をデバウンス付きで実行し、結果を表示する。
type GrepModel struct {
	textInput   textinput.Model
	items       []string // "file:line:text" 形式
	cursor      int
	loading     bool
	err         error
	debounceTag int
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
	case tea.KeyUp:
		if g.cursor > 0 {
			g.cursor--
		}
		return g, nil
	case tea.KeyDown:
		if len(g.items) > 0 && g.cursor < len(g.items)-1 {
			g.cursor++
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

func (g *GrepModel) View() string {
	var b strings.Builder
	for i, item := range g.items {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == g.cursor {
			b.WriteString(selectedItemStyle.Render("> " + item))
		} else {
			b.WriteString(normalItemStyle.Render("  " + item))
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

// TextInput は入力欄の View を返す（親 Model がヘッダーに組み込む用）。
func (g *GrepModel) TextInput() textinput.Model { return g.textInput }

// Reset はモード切替時にペインの状態をリセットする。
func (g *GrepModel) Reset() {
	g.textInput.SetValue("")
	g.cursor = 0
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
