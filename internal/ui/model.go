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
		// エディタ起動失敗（LookPath 失敗 / exec 失敗 / 非0 終了）を active pane の
		// ステータス行に surface する（#010）。成功時は他の状態に触れない。
		if msg.Err != nil {
			m.activePane().SetErr(msg.Err)
		}
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
// Selector ロールを実装しないペインは Enter を no-op として扱う（#006）。
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	sel, ok := m.activePane().(Selector)
	if !ok {
		return m, nil
	}
	file, line := sel.OpenTarget()
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
// Selector / PreviewDecorator を実装しないペインはプレビュー機能なしとして扱う（#006）。
func (m *Model) previewCmd() tea.Cmd {
	pane := m.activePane()
	sel, ok := pane.(Selector)
	if !ok {
		m.previewContent = ""
		m.previewPath = ""
		m.previewStartLine = 0
		return nil
	}
	filePath := sel.FilePath()
	if filePath == "" {
		m.previewContent = ""
		m.previewPath = ""
		m.previewStartLine = 0
		return nil
	}
	visibleHeight := m.contentHeight()
	startLine := 1
	if dec, ok := pane.(PreviewDecorator); ok {
		startLine = dec.PreviewRange(visibleHeight)
	}
	if filePath == m.previewPath && startLine == m.previewStartLine {
		return nil
	}
	m.previewPath = filePath
	m.previewStartLine = startLine
	return func() tea.Msg {
		if tooLarge, err := preview.IsTooLarge(filePath); err == nil && tooLarge {
			return PreviewLoadedMsg{Content: preview.LargeFileMessage, Path: filePath}
		}
		if binary, err := preview.IsBinary(filePath); err == nil && binary {
			return PreviewLoadedMsg{Content: preview.BinaryFileMessage, Path: filePath}
		}
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
// 引数組み立ては buildEditorArgs に委譲し、エディタ別の形式差と
// 悪意あるファイル名（`-` `+` 始まり）によるフラグ注入を吸収する。
// LookPath で事前検証を行い、エディタが PATH 上に無いケースを
// その場で EditorFinishedMsg{Err: ...} として表面化する（#010）。
// ExecProcess に到達してから「何も起きない」体験を防ぐ目的。
func openEditorCmd(file string, line int) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	if _, err := exec.LookPath(editor); err != nil {
		return func() tea.Msg {
			return EditorFinishedMsg{Err: fmt.Errorf("editor %q not found: %w", editor, err)}
		}
	}
	args := buildEditorArgs(editor, file, line)
	c := exec.Command(editor, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

// View は画面を描画する。
func (m Model) View() string {
	pane := m.activePane()

	// ヘッダー: pane が提供する完成済みの行をそのまま縦に並べる（proposal #001 D-3 / D-4）。
	// モードラベル "[Grep]" / "[Files]" や入力欄ラベル "files:" は pane が責任を持って描画し、
	// active/inactive のスタイル切替もここに含めて返す。
	// HeaderRenderer 未実装のペインは入力欄なしで描画（将来 Buffer List 等への退路 / #006）。
	headerInputs := []string{""}
	if hr, ok := pane.(HeaderRenderer); ok {
		headerInputs = hr.HeaderViews()
	}
	if len(headerInputs) == 0 {
		headerInputs = []string{""}
	}
	headerLines := make([]string, len(headerInputs))
	for i, inputView := range headerInputs {
		line := headerStyle.Render(inputView)
		if m.width > 0 {
			line = ansi.Truncate(line, m.width, "")
		}
		headerLines[i] = line
	}
	header := strings.Join(headerLines, "\n")

	// エラーはステータス行として header 直下に出す（#009）。
	// 早期 return をせず通常レイアウトを維持し、修正のためのキー入力を続けられるようにする。
	// rg の stderr は複数行（例: "regex parse error:\n    [\n    ^\nerror: ..."）
	// で返るため、後段で実際の行数を contentHeight から差し引く。
	errorLine := ""
	errorRows := 0
	if e := pane.Err(); e != nil {
		errorLine = errorStyle.Render(fmt.Sprintf("error: %s", e.Error())) + "\n"
		errorRows = strings.Count(errorLine, "\n")
	}

	// レイアウト計算
	// NormalBorder は上下左右各1文字分を消費する（左右で+2、上下で+2）
	borderH := 2
	borderW := 2
	contentHeight := m.contentHeight()
	// header が複数行になった分とエラー行の分だけペイン高さを縮める。
	// これを忘れると総行数が m.height を超え、altscreen のカーソル確保で
	// 画面が上にスクロールし header (textinput) が画面外に出る。
	extraHeaderLines := len(headerLines) - 1
	if extraHeaderLines > 0 {
		contentHeight -= extraHeaderLines
	}
	if errorRows > 0 {
		contentHeight -= errorRows
	}
	if contentHeight < 1 {
		contentHeight = 1
	}
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

	// プレビューコンテンツを表示サイズに切り詰め + ペイン固有の装飾。
	// PreviewDecorator 未実装のペインは無装飾で素通し（#006）。
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
		if dec, ok := pane.(PreviewDecorator); ok {
			previewText = dec.DecoratePreview(previewText, previewWidth)
		}
	}

	rightPane := previewPaneStyle.
		Width(previewWidth).
		MaxWidth(previewWidth + borderW).
		Height(contentHeight).
		MaxHeight(contentHeight + borderH).
		Render(previewText)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	return header + "\n" + errorLine + body
}
