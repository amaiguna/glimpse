package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/amaiguna/telescope-tui/internal/grep"
	"github.com/amaiguna/telescope-tui/internal/preview"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Mode はファインダーの動作モードを表す。
type Mode int

const (
	// ModeFinder はファイルファインダーモード。
	ModeFinder Mode = iota
	// ModeGrep はライブ grep モード。
	ModeGrep
)

// --- カスタム Msg（共有） ---

// FilesLoadedMsg はファイル列挙が完了したことを通知する。
type FilesLoadedMsg struct {
	Items []string
}

// FilesErrorMsg はファイル列挙中にエラーが発生したことを通知する。
type FilesErrorMsg struct {
	Err error
}

// GrepDoneMsg は grep 検索が完了したことを通知する。
type GrepDoneMsg struct {
	Matches []grep.Match
}

// GrepErrorMsg は grep 検索中にエラーが発生したことを通知する。
type GrepErrorMsg struct {
	Err error
}

// EditorFinishedMsg はエディタプロセスが終了したことを通知する。
type EditorFinishedMsg struct {
	Err error
}

// debounceTickMsg はデバウンスタイマーが発火したことを通知する。
type debounceTickMsg struct {
	tag int
}

// previewMaxLines はプレビューペインに表示する最大行数。
const previewMaxLines = 50

// Model は telescope-tui の親モデル。
// アクティブな Pane にメッセージをルーティングし、レイアウトを管理する。
type Model struct {
	mode           Mode
	finderPane     *FinderModel
	grepPane       *GrepModel
	width          int
	height         int
	previewContent string
}

// NewModel は Model を初期化して返す。
func NewModel() Model {
	return Model{
		mode:       ModeFinder,
		finderPane: NewFinderModel(),
		grepPane:   NewGrepModel(),
	}
}

// activePane はアクティブなペインを返す。
func (m *Model) activePane() Pane {
	if m.mode == ModeGrep {
		return m.grepPane
	}
	return m.finderPane
}

// Init は初期化時にファイル列挙コマンドを発行する。
func (m Model) Init() tea.Cmd {
	return loadFilesCmd()
}

// Update はメッセージを処理する。グローバルキーを処理し、それ以外はアクティブなペインに委譲する。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEscape:
			return m, tea.Quit
		case tea.KeyTab:
			return m.switchMode()
		case tea.KeyEnter:
			return m.handleEnter()
		default:
			return m.delegateToPane(msg)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case EditorFinishedMsg:
		return m, nil
	default:
		return m.delegateToPane(msg)
	}
}

// delegateToPane はアクティブなペインにメッセージを委譲し、プレビューを更新する。
func (m Model) delegateToPane(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.mode == ModeGrep {
		_, cmd = m.grepPane.Update(msg)
	} else {
		_, cmd = m.finderPane.Update(msg)
	}
	m.updatePreview()
	return m, cmd
}

// switchMode は Finder ↔ Grep のモード切替を行う。
func (m Model) switchMode() (tea.Model, tea.Cmd) {
	if m.mode == ModeFinder {
		m.mode = ModeGrep
		m.grepPane.Reset()
	} else {
		m.mode = ModeFinder
		m.finderPane.Reset()
	}
	m.updatePreview()
	return m, nil
}

// handleEnter は選択アイテムでエディタを起動する。
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	pane := m.activePane()
	selected := pane.SelectedItem()
	if selected == "" {
		return m, nil
	}

	switch m.mode {
	case ModeFinder:
		return m, openEditorCmd(selected, 0)
	case ModeGrep:
		file, line := parseGrepItem(selected)
		return m, openEditorCmd(file, line)
	}
	return m, nil
}

// updatePreview は現在のアクティブペインのファイルパスに基づいてプレビューを更新する。
func (m *Model) updatePreview() {
	filePath := m.activePane().FilePath()
	if filePath == "" {
		m.previewContent = ""
		return
	}

	content, err := preview.ReadFile(filePath, previewMaxLines)
	if err != nil {
		m.previewContent = fmt.Sprintf("error: %s", err.Error())
		return
	}

	highlighted, err := preview.Highlight(filePath, content)
	if err != nil {
		m.previewContent = content
		return
	}
	m.previewContent = highlighted
}

// openEditorCmd は $EDITOR でファイルを開くコマンドを返す。
func openEditorCmd(file string, line int) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	var args []string
	if line > 0 {
		args = append(args, fmt.Sprintf("+%d", line))
	}
	args = append(args, file)
	c := exec.Command(editor, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

// View は画面を描画する。
func (m Model) View() string {
	pane := m.activePane()

	// モードラベル + クエリ入力行
	modeLabel := "Files"
	if m.mode == ModeGrep {
		modeLabel = "Grep"
	}
	header := fmt.Sprintf("[%s] > %s", modeLabel, pane.Query())

	// エラー表示
	if pane.Err() != nil {
		return header + "\n" + fmt.Sprintf("error: %s", pane.Err().Error()) + "\n"
	}

	// ローディング表示
	if pane.IsLoading() {
		return header + "\n" + "loading...\n"
	}

	// レイアウト計算
	separatorWidth := 3
	listWidth := m.width/2 - separatorWidth/2
	if listWidth < 20 {
		listWidth = 20
	}
	previewWidth := m.width - listWidth - separatorWidth
	if previewWidth < 0 {
		previewWidth = 0
	}
	contentHeight := m.height - 2
	if contentHeight < 1 {
		contentHeight = 10
	}

	// 各ペインを lipgloss でスタイル化
	listStyle := lipgloss.NewStyle().
		Width(listWidth).
		MaxWidth(listWidth).
		Height(contentHeight).
		MaxHeight(contentHeight)

	separatorStyle := lipgloss.NewStyle().
		Width(separatorWidth).
		MaxWidth(separatorWidth).
		Height(contentHeight).
		MaxHeight(contentHeight)

	previewStyle := lipgloss.NewStyle().
		Width(previewWidth).
		MaxWidth(previewWidth).
		Height(contentHeight).
		MaxHeight(contentHeight)

	// セパレータ
	var sepLines []string
	for i := 0; i < contentHeight; i++ {
		sepLines = append(sepLines, " | ")
	}
	separatorContent := strings.Join(sepLines, "\n")

	// レンダリングと結合
	leftPane := listStyle.Render(pane.View())
	sep := separatorStyle.Render(separatorContent)
	rightPane := previewStyle.Render(m.previewContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, sep, rightPane)

	return header + "\n" + body
}
