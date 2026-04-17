package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/amaiguna/telescope-tui/internal/finder"
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

// debounceInterval は Grep モードでの入力デバウンス間隔。
const debounceInterval = 300 * time.Millisecond

// --- カスタム Msg ---

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
	tag int // どのデバウンスリクエストに対応するか識別するタグ
}

// previewMaxLines はプレビューペインに表示する最大行数。
const previewMaxLines = 50

// Model は telescope-tui のメイン UI モデル。
type Model struct {
	mode           Mode
	query          string
	items          []string // 表示中のアイテム（Finder: ファイルパス, Grep: "file:line:text"）
	allFiles       []string // Finder モードの全ファイルリスト（フィルタ前）
	cursor         int
	width          int
	height         int
	err            error
	loading        bool
	debounceTag    int    // 現在のデバウンスタグ（Grep モード用）
	previewContent string // 現在のプレビュー表示内容（ハイライト済み）
}

func NewModel() Model {
	return Model{
		mode: ModeFinder,
	}
}

// Init は初期化時にファイル列挙コマンドを発行する。
func (m Model) Init() tea.Cmd {
	return loadFilesCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case FilesLoadedMsg:
		m.loading = false
		m.err = nil
		m.allFiles = msg.Items
		m.applyFilter()
		m.updatePreview()
	case FilesErrorMsg:
		m.loading = false
		m.err = msg.Err
	case GrepDoneMsg:
		m.loading = false
		m.err = nil
		m.items = formatGrepMatches(msg.Matches)
		m.cursor = 0
		m.updatePreview()
	case GrepErrorMsg:
		m.loading = false
		m.err = msg.Err
	case debounceTickMsg:
		return m.handleDebounceTick(msg)
	case EditorFinishedMsg:
		m.err = msg.Err
	}
	return m, nil
}

// handleKeyMsg はキー入力メッセージを処理する。
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEscape:
		return m, tea.Quit

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.updatePreview()
		}
		return m, nil

	case tea.KeyDown:
		if len(m.items) > 0 && m.cursor < len(m.items)-1 {
			m.cursor++
			m.updatePreview()
		}
		return m, nil

	case tea.KeyEnter:
		return m.handleEnter()

	case tea.KeyTab:
		return m.switchMode()

	case tea.KeyBackspace:
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
			m.applyFilter()
		}
		return m, nil

	case tea.KeyRunes:
		m.query += string(msg.Runes)
		return m.handleQueryChange()

	default:
		return m, nil
	}
}

// switchMode は Finder ↔ Grep のモード切替を行う。
func (m Model) switchMode() (tea.Model, tea.Cmd) {
	if m.mode == ModeFinder {
		m.mode = ModeGrep
	} else {
		m.mode = ModeFinder
	}
	m.query = ""
	m.cursor = 0
	m.items = nil
	m.err = nil

	if m.mode == ModeFinder {
		// Finder に戻ったら全ファイルを表示
		m.items = m.allFiles
	}
	return m, nil
}

// handleQueryChange はクエリ変更時の処理を行う。
func (m Model) handleQueryChange() (tea.Model, tea.Cmd) {
	switch m.mode {
	case ModeFinder:
		m.applyFilter()
		return m, nil
	case ModeGrep:
		// デバウンス: タグをインクリメントしてタイマー発火
		m.debounceTag++
		tag := m.debounceTag
		return m, tea.Tick(debounceInterval, func(time.Time) tea.Msg {
			return debounceTickMsg{tag: tag}
		})
	}
	return m, nil
}

// applyFilter は Finder モードでファジーフィルタを適用する。
func (m *Model) applyFilter() {
	filtered := finder.FuzzyFilter(m.query, m.allFiles)
	if filtered == nil {
		m.items = nil
	} else {
		m.items = make([]string, len(filtered))
		for i, f := range filtered {
			m.items[i] = f.Str
		}
	}
	// カーソルをクランプ
	if len(m.items) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	m.updatePreview()
}

// handleEnter は選択アイテムでエディタを起動する。
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	if len(m.items) == 0 {
		return m, nil
	}
	item := m.items[m.cursor]

	switch m.mode {
	case ModeFinder:
		return m, openEditorCmd(item, 0)
	case ModeGrep:
		file, line := parseGrepItem(item)
		return m, openEditorCmd(file, line)
	}
	return m, nil
}

// handleDebounceTick はデバウンスタイマー発火時の処理を行う。
func (m Model) handleDebounceTick(msg debounceTickMsg) (tea.Model, tea.Cmd) {
	if msg.tag != m.debounceTag {
		return m, nil
	}
	if m.query == "" {
		m.items = nil
		return m, nil
	}
	m.loading = true
	return m, runGrepCmd(m.query)
}

// --- Cmd 関数 ---

// loadFilesCmd はファイル列挙を非同期で実行するコマンドを返す。
func loadFilesCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := finder.ListFiles()
		if err != nil {
			return FilesErrorMsg{Err: err}
		}
		return FilesLoadedMsg{Items: items}
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

// openEditorCmd は $EDITOR でファイルを開くコマンドを返す。
// line が 0 より大きい場合は +line 引数を付ける。
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

// updatePreview は現在のカーソル位置に対応するファイルのプレビューを更新する。
func (m *Model) updatePreview() {
	if len(m.items) == 0 {
		m.previewContent = ""
		return
	}

	item := m.items[m.cursor]
	var filePath string
	switch m.mode {
	case ModeFinder:
		filePath = item
	case ModeGrep:
		filePath, _ = parseGrepItem(item)
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

// View は画面を描画する。
func (m Model) View() string {
	// モードラベル + クエリ入力行
	modeLabel := "Files"
	if m.mode == ModeGrep {
		modeLabel = "Grep"
	}
	header := fmt.Sprintf("[%s] > %s", modeLabel, m.query)

	// エラー表示
	if m.err != nil {
		return header + "\n" + fmt.Sprintf("error: %s", m.err.Error()) + "\n"
	}

	// ローディング表示
	if m.loading {
		return header + "\n" + "loading...\n"
	}

	// レイアウト計算
	separatorWidth := 3 // " | "
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

	// リストペインの内容を構築
	var listContent strings.Builder
	for i, item := range m.items {
		if i >= contentHeight {
			break
		}
		if i > 0 {
			listContent.WriteString("\n")
		}
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		listContent.WriteString(cursor + item)
	}

	// 各ペインを lipgloss でスタイル化（固定幅・固定高さ）
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

	// セパレータを構築（各行に " | "）
	var sepLines []string
	for i := 0; i < contentHeight; i++ {
		sepLines = append(sepLines, " | ")
	}
	separatorContent := strings.Join(sepLines, "\n")

	// 各ペインをレンダリング
	leftPane := listStyle.Render(listContent.String())
	sep := separatorStyle.Render(separatorContent)
	rightPane := previewStyle.Render(m.previewContent)

	// 左右を結合
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, sep, rightPane)

	return header + "\n" + body
}
