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
	"github.com/charmbracelet/x/ansi"
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
		m.updatePaneSizes()
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
		m.finderPane.Blur()
		m.grepPane.Reset()
		cmd := m.grepPane.Focus()
		m.updatePreview()
		return m, cmd
	}
	m.mode = ModeFinder
	m.grepPane.Blur()
	m.finderPane.Reset()
	cmd := m.finderPane.Focus()
	m.updatePreview()
	return m, cmd
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

// contentHeight はペイン内部の表示可能行数を計算する。
func (m *Model) contentHeight() int {
	// ヘッダー1行 + 枠線上下2行分を引く
	h := m.height - 2 - 2
	if h < 1 {
		h = 10
	}
	return h
}

// listWidth はリストペインの内部幅を計算する。
func (m *Model) listWidth() int {
	borderW := 2
	w := (m.width*3)/10 - borderW
	if w < 10 {
		w = 10
	}
	return w
}

// updatePaneSizes は両ペインの表示可能サイズを更新する。
func (m *Model) updatePaneSizes() {
	h := m.contentHeight()
	w := m.listWidth()
	m.finderPane.SetViewSize(h, w)
	m.grepPane.SetViewSize(h, w)
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
	var inputView string
	if m.mode == ModeGrep {
		inputView = m.grepPane.TextInput().View()
	} else {
		inputView = m.finderPane.TextInput().View()
	}
	header := headerStyle.Render(modeLabelStyle.Render("["+modeLabel+"]") + " " + inputView)

	// エラー表示
	if pane.Err() != nil {
		return header + "\n" + errorStyle.Render(fmt.Sprintf("error: %s", pane.Err().Error())) + "\n"
	}

	// ローディング表示
	if pane.IsLoading() {
		return header + "\n" + loadingStyle.Render("loading...") + "\n"
	}

	// レイアウト計算
	// NormalBorder は上下左右各1文字分を消費する（左右で+2、上下で+2）
	borderH := 2
	borderW := 2
	contentHeight := m.contentHeight()
	listWidth := m.listWidth()
	previewWidth := m.width - listWidth - borderW*2
	if previewWidth < 0 {
		previewWidth = 0
	}

	// 各ペインを枠線付きでレンダリング
	leftPane := listPaneStyle.
		Width(listWidth).
		MaxWidth(listWidth + borderW).
		Height(contentHeight).
		MaxHeight(contentHeight + borderH).
		Render(pane.View())

	// プレビューコンテンツを表示サイズに切り詰め
	previewText := m.previewContent
	if previewText != "" {
		lines := strings.Split(previewText, "\n")
		if len(lines) > contentHeight {
			lines = lines[:contentHeight]
		}
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, previewWidth, "")
		}
		previewText = strings.Join(lines, "\n")
	}

	rightPane := previewPaneStyle.
		Width(previewWidth).
		MaxWidth(previewWidth + borderW).
		Height(contentHeight).
		MaxHeight(contentHeight + borderH).
		Render(previewText)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	return header + "\n" + body
}
