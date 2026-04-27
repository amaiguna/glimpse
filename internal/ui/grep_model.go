package ui

import (
	"context"
	"errors"
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
	// searchCancel は進行中の rg 検索を途中で kill するための CancelFunc。
	// 新しい debounce 発火時や Reset 時に呼び出して古いプロセスを回収する（M-3）。
	searchCancel context.CancelFunc
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

// grepSearchTimeout は 1 回の grep 検索に許す最大時間。
// これを超えると rg プロセスが kill され GrepErrorMsg（DeadlineExceeded）が返る。
const grepSearchTimeout = 10 * time.Second

// runGrepCmd は rg 検索を非同期で実行するコマンドを返す。
// ctx のキャンセルで古い rg プロセスを kill し stdout の溜め込みを防ぐ（M-3）。
// キャンセル（context.Canceled）は新しい検索で上書き中なので UI に伝えず無視する。
// DeadlineExceeded は明示的なタイムアウトなのでエラー表示する。
func runGrepCmd(ctx context.Context, pattern string) tea.Cmd {
	return func() tea.Msg {
		matches, err := grep.Search(ctx, pattern)
		if errors.Is(err, context.Canceled) {
			return nil
		}
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
		g.err = simplifyGrepError(msg.Err)
		// items は維持する（broken regex 入力中に前回ヒットを消さない / #007）。
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
	// 前回の検索が残っていれば cancel（rg プロセス kill + stdout 溜め込み回避）。
	if g.searchCancel != nil {
		g.searchCancel()
	}
	if g.textInput.Value() == "" {
		g.searchCancel = nil
		g.items = nil
		// クエリ空 = idle 遷移なので、前回の regex エラーは陳腐化する。
		// 一緒にクリアして UI 上に古いエラーが残らないようにする（#007 取りこぼし）。
		g.err = nil
		return g, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), grepSearchTimeout)
	g.searchCancel = cancel
	g.loading = true
	return g, runGrepCmd(ctx, g.textInput.Value())
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

func (g *GrepModel) Query() string    { return g.textInput.Value() }
func (g *GrepModel) IsLoading() bool  { return g.loading }
func (g *GrepModel) Err() error       { return g.err }
func (g *GrepModel) SetErr(err error) { g.err = err }

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
// 進行中の rg 検索があれば cancel してプロセスを回収する（M-3）。
func (g *GrepModel) Reset() {
	if g.searchCancel != nil {
		g.searchCancel()
		g.searchCancel = nil
	}
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

// simplifyGrepError は UI 表示用にエラーメッセージを整形する（#007）。
// rg が non-zero exit + stderr を返したケース（broken regex など）では、
// "exit status 2:" の冗長プレフィックスを落として stderr 本文だけを surface する。
// stderr が空、または CmdError ではない場合は元のエラーをそのまま返す。
func simplifyGrepError(err error) error {
	if err == nil {
		return nil
	}
	var cmdErr *grep.CmdError
	if errors.As(err, &cmdErr) && strings.TrimSpace(cmdErr.Stderr) != "" {
		return errors.New(strings.TrimSpace(cmdErr.Stderr))
	}
	return err
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

// parseGrepItem は "file:line:text" 形式の文字列からファイルパスと行番号を抽出する（I-2）。
// 「`:` の直後が `\d+` で、その後が `:` または文末」というパターンを左から走査するため、
// ファイルパス側に `:` を含む（Windows `C:\foo:10:hit` や `weird:name.txt:5:hit`）ケース、
// および text 側に `:` を含む（`main.go:42:foo:bar`）ケース両方に正しく動作する。
// 形式不一致や行番号非数値の場合は (item, 0) を返す。
func parseGrepItem(item string) (string, int) {
	for i := 0; i < len(item); i++ {
		if item[i] != ':' {
			continue
		}
		j := i + 1
		for j < len(item) && item[j] >= '0' && item[j] <= '9' {
			j++
		}
		if j == i+1 {
			continue // `:` 直後が数字でない
		}
		if j < len(item) && item[j] != ':' {
			continue // 数字の後が `:` でも文末でもない
		}
		line, err := strconv.Atoi(item[i+1 : j])
		if err != nil || line <= 0 {
			// 行番号は 1-based。0 や負の値は parse 失敗扱い（fuzz 検出: ":00" 等）。
			continue
		}
		return item[:i], line
	}
	return item, 0
}
