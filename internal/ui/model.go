package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/amaiguna/glimpse-tui/internal/grep"
	"github.com/amaiguna/glimpse-tui/internal/preview"
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

// PaneTarget は Finder ペインを返す。
func (FilesLoadedMsg) PaneTarget() Mode { return ModeFinder }

// FilesErrorMsg はファイル列挙中にエラーが発生したことを通知する。
type FilesErrorMsg struct {
	Err error
}

// PaneTarget は Finder ペインを返す。
func (FilesErrorMsg) PaneTarget() Mode { return ModeFinder }

// GrepDoneMsg は grep 検索が完了したことを通知する。
type GrepDoneMsg struct {
	Matches []grep.Match
}

// PaneTarget は Grep ペインを返す。
func (GrepDoneMsg) PaneTarget() Mode { return ModeGrep }

// GrepErrorMsg は grep 検索中にエラーが発生したことを通知する。
type GrepErrorMsg struct {
	Err error
}

// PaneTarget は Grep ペインを返す。
func (GrepErrorMsg) PaneTarget() Mode { return ModeGrep }

// PreviewLoadedMsg はプレビューの非同期読み込みが完了したことを通知する。
type PreviewLoadedMsg struct {
	Content string
	Path    string // 古いプレビューが上書きされないようパスを照合
}

// EditorFinishedMsg はエディタプロセスが終了したことを通知する。
type EditorFinishedMsg struct {
	Err error
}

// debounceTickMsg はデバウンスタイマーが発火したことを通知する。
type debounceTickMsg struct {
	tag int
}

// PaneTarget は Grep ペインを返す。
func (debounceTickMsg) PaneTarget() Mode { return ModeGrep }

// Model は glimpse-tui の親モデル。
// アクティブな Pane にメッセージをルーティングし、レイアウトを管理する。
type Model struct {
	mode             Mode
	finderPane       *FinderModel
	grepPane         *GrepModel
	width            int
	height           int
	previewContent   string
	previewPath      string // 現在プレビュー中のファイルパス（照合用）
	previewStartLine int    // 現在プレビュー中の開始行（照合用）
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
	case PreviewLoadedMsg:
		// 古いプレビューが後から届いた場合は無視
		if msg.Path == m.previewPath {
			m.previewContent = msg.Content
		}
		return m, nil
	case EditorFinishedMsg:
		return m, nil

	// ペイン固有 Msg → PaneTarget() で宛先を判別
	case paneMsg:
		switch msg.PaneTarget() {
		case ModeGrep:
			return m.delegateToGrep(msg)
		default:
			return m.delegateToFinder(msg)
		}

	default:
		return m.delegateToPane(msg)
	}
}

// delegateToFinder は FinderPane にメッセージを委譲する。
func (m Model) delegateToFinder(msg tea.Msg) (tea.Model, tea.Cmd) {
	pane, cmd := m.finderPane.Update(msg)
	m.finderPane = pane.(*FinderModel)
	return m, tea.Batch(cmd, m.previewCmd())
}

// delegateToGrep は GrepPane にメッセージを委譲する。
func (m Model) delegateToGrep(msg tea.Msg) (tea.Model, tea.Cmd) {
	pane, cmd := m.grepPane.Update(msg)
	m.grepPane = pane.(*GrepModel)
	return m, tea.Batch(cmd, m.previewCmd())
}

// delegateToPane はアクティブなペインにメッセージを委譲し、プレビューを更新する。
func (m Model) delegateToPane(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.mode == ModeGrep {
		pane, c := m.grepPane.Update(msg)
		m.grepPane = pane.(*GrepModel)
		cmd = c
	} else {
		pane, c := m.finderPane.Update(msg)
		m.finderPane = pane.(*FinderModel)
		cmd = c
	}
	return m, tea.Batch(cmd, m.previewCmd())
}

// switchMode は Finder ↔ Grep のモード切替を行う。
func (m Model) switchMode() (tea.Model, tea.Cmd) {
	if m.mode == ModeFinder {
		m.mode = ModeGrep
		m.finderPane.Blur()
		m.grepPane.Reset()
		cmd := m.grepPane.Focus()
		return m, tea.Batch(cmd, m.previewCmd())
	}
	m.mode = ModeFinder
	m.grepPane.Blur()
	m.finderPane.Reset()
	cmd := m.finderPane.Focus()
	return m, tea.Batch(cmd, m.previewCmd())
}

// handleEnter は選択アイテムでエディタを起動する。
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	pane := m.activePane()
	file, line := pane.OpenTarget()
	if file == "" {
		return m, nil
	}
	return m, openEditorCmd(file, line)
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

// previewCmd は現在のアクティブペインのファイルパスに基づいてプレビュー読み込みの Cmd を返す。
// ファイルパスまたは表示開始行が変わっていない場合は Cmd を発行しない。
func (m *Model) previewCmd() tea.Cmd {
	pane := m.activePane()
	filePath := pane.FilePath()
	if filePath == "" {
		m.previewContent = ""
		m.previewPath = ""
		m.previewStartLine = 0
		return nil
	}
	visibleHeight := m.contentHeight()
	startLine := pane.PreviewRange(visibleHeight)
	if filePath == m.previewPath && startLine == m.previewStartLine {
		return nil
	}
	m.previewPath = filePath
	m.previewStartLine = startLine
	return func() tea.Msg {
		content, err := preview.ReadFileRange(filePath, startLine, visibleHeight)
		if err != nil {
			return PreviewLoadedMsg{
				Content: fmt.Sprintf("error: %s", err.Error()),
				Path:    filePath,
			}
		}
		highlighted, err := preview.Highlight(filePath, content)
		if err != nil {
			highlighted = content
		}
		return PreviewLoadedMsg{Content: highlighted, Path: filePath}
	}
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
	inputView := pane.TextInputView()
	headerText := modeLabelStyle.Render("["+modeLabel+"]") + " " + inputView
	header := headerStyle.Render(headerText)
	if m.width > 0 {
		header = ansi.Truncate(header, m.width, "")
	}

	// エラー表示（枠線なしで早期リターン）
	if pane.Err() != nil {
		return header + "\n" + errorStyle.Render(fmt.Sprintf("error: %s", pane.Err().Error())) + "\n"
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

	// プレビューコンテンツを表示サイズに切り詰め + ペイン固有の装飾
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
		previewText = pane.DecoratePreview(previewText, previewWidth)
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
