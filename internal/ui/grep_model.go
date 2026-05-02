package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amaiguna/glimpse-tui/internal/finder"
	"github.com/amaiguna/glimpse-tui/internal/grep"
	"github.com/amaiguna/glimpse-tui/internal/sanitize"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// debounceInterval は Grep モードでの入力デバウンス間隔。
const debounceInterval = 100 * time.Millisecond

// grepFocusedInput は GrepModel 内のどの入力欄にフォーカスがあるかを表す（proposal #001 Phase 2）。
type grepFocusedInput int

const (
	// grepInputPattern は grep パターン入力欄（1 行目）にフォーカスがある状態。
	grepInputPattern grepFocusedInput = iota
	// grepInputInclude は include glob 入力欄（2 行目）にフォーカスがある状態。
	grepInputInclude
)

// GrepModel はライブ grep モードのペイン。
// rg --json をデバウンス付きで実行し、結果を表示する。
// proposal #001 から「対象ファイル絞り込み (include)」用の第 2 入力欄を持ち、
// Shift+Tab で 2 入力欄間の focus を移動できる。
// include 入力は allFiles (Finder と共有のファイル list) に対する fuzzy filter として動作し、
// マッチしたファイル群だけが rg の検索対象になる (proposal D-2(b) fuzzy 路線)。
type GrepModel struct {
	textInput        textinput.Model // grep パターン入力欄
	includeInput     textinput.Model // include 用 fuzzy filter 入力欄
	focusedInput     grepFocusedInput
	items            []string // "file:line:text" 形式
	cursor           int
	offset           int // スクロールオフセット（表示先頭行）
	viewHeight       int // 表示可能行数（親から設定）
	viewWidth        int // 表示可能幅（親から設定）
	loading          bool
	err              error
	debounceTag      int
	previewStartLine int // PreviewRange が最後に返した開始行（DecoratePreview で使用）
	// allFiles は Finder と共有のファイル列挙結果 (FilesLoadedMsg 経由で SetAllFiles される)。
	// include 入力欄に値があるとき、この list を fuzzy filter して rg に渡す。
	allFiles []string
	// pathMatchedIndexes は include の fuzzy filter で各パスにマッチした rune 位置のマップ
	// (proposal #002 Phase 3)。include 非空時のみ populate され、View で各 item の
	// ファイルパス部分のハイライト ANSI 挿入に使う。include 空時 / Reset 時は nil。
	pathMatchedIndexes map[string][]int
	// searchCancel は進行中の rg 検索を途中で kill するための CancelFunc。
	// 新しい debounce 発火時や Reset 時に呼び出して古いプロセスを回収する（M-3）。
	searchCancel context.CancelFunc
}

// includeInputPlaceholder は include 入力欄の placeholder 文言。
// fuzzy filter であることを示し、何が入るか (ファイルパス) を hint する。
// "files: " ラベルは HeaderViews 側で別途付加するため、ここには含めない。
const includeInputPlaceholder = "filter files (fuzzy)..."

// NewGrepModel は GrepModel を初期化して返す。
func NewGrepModel() *GrepModel {
	ti := textinput.New()
	ti.Placeholder = "Search pattern..."
	ti.Focus()
	ti.CharLimit = 256

	include := textinput.New()
	include.Placeholder = includeInputPlaceholder
	include.CharLimit = 256

	return &GrepModel{
		textInput:    ti,
		includeInput: include,
		focusedInput: grepInputPattern,
	}
}

// grepSearchTimeout は 1 回の grep 検索に許す最大時間。
// これを超えると rg プロセスが kill され GrepErrorMsg（DeadlineExceeded）が返る。
const grepSearchTimeout = 10 * time.Second

// runGrepCmd は rg 検索を非同期で実行するコマンドを返す。
// ctx のキャンセルで古い rg プロセスを kill し stdout の溜め込みを防ぐ（M-3）。
// キャンセル（context.Canceled）は新しい検索で上書き中なので UI に伝えず無視する。
// DeadlineExceeded は明示的なタイムアウトなのでエラー表示する。
// files は include 入力欄から fuzzyFilterFiles で絞り込んだファイル群。nil なら全検索。
func runGrepCmd(ctx context.Context, pattern string, files []string) tea.Cmd {
	return func() tea.Msg {
		matches, err := grep.Search(ctx, pattern, files)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return GrepErrorMsg{Err: err}
		}
		return GrepDoneMsg{Matches: matches}
	}
}

// fuzzyFilterFiles は include 入力欄の値で allFiles を fuzzy フィルタし、
// マッチしたパス list と「パス → MatchedIndexes」マップの両方を返す
// (proposal #001 D-2(b) fuzzy 路線 / proposal #002 Phase 3)。
//
//   - query が空 → (nil, nil) — filter 無効
//   - files が空 → (nil, nil) — filter 元が無い
//   - マッチ 0 件 → (nil, nil) — caller は rg を呼ばない
//   - マッチあり → (パス list, パス→indexes マップ)
//
// パス list は rg の検索対象として渡す。indexes マップは Grep View で
// 各 item のファイルパス部分にハイライト ANSI を挿入するために使う (proposal #002 D-2)。
//
// rg の `--glob` を使わずアプリ側で絞り込みすることで、rg の「`--glob` は ignore を
// 上書きする」仕様 (`.git/` まで掘ってしまう問題) を回避し、Finder 側の fuzzy 体験と
// 完全に揃える。
func fuzzyFilterFiles(query string, files []string) ([]string, map[string][]int) {
	if query == "" || len(files) == 0 {
		return nil, nil
	}
	matches := finder.FuzzyFilter(query, files)
	if len(matches) == 0 {
		return nil, nil
	}
	paths := make([]string, len(matches))
	indexes := make(map[string][]int, len(matches))
	for i, m := range matches {
		paths[i] = m.Str
		indexes[m.Str] = m.MatchedIndexes
	}
	return paths, indexes
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
	case tea.KeyShiftTab:
		// proposal #001 D-3: Shift+Tab で pattern ↔ include 入力欄の focus を移動。
		// グローバル Tab (モード切替) はそのまま温存し、ペイン内の局所遷移として扱う。
		return g.toggleInputFocus()
	default:
		// テキスト入力は focus 中の入力欄に委譲。
		// proposal #001 Phase 3: pattern / include どちらの入力でも debounce を発火させる。
		// 値が変わらないキー (矢印は別 case で処理済み、Home/End 等) では debounce を起こさない。
		if g.focusedInput == grepInputInclude {
			prev := g.includeInput.Value()
			var cmd tea.Cmd
			g.includeInput, cmd = g.includeInput.Update(msg)
			if g.includeInput.Value() != prev {
				return g, tea.Batch(cmd, g.scheduleDebounce())
			}
			return g, cmd
		}
		prevQuery := g.textInput.Value()
		var cmd tea.Cmd
		g.textInput, cmd = g.textInput.Update(msg)
		if g.textInput.Value() != prevQuery {
			return g, tea.Batch(cmd, g.scheduleDebounce())
		}
		return g, cmd
	}
}

// scheduleDebounce は debounce タイマーを 1 つ発火させる Cmd を返す（proposal #001 Phase 3）。
// debounceTag をインクリメントして「最新だけ走らせる」契約を保つ。
// pattern / include どちらの入力でも共通に呼び出す共通化。
func (g *GrepModel) scheduleDebounce() tea.Cmd {
	g.debounceTag++
	tag := g.debounceTag
	return tea.Tick(debounceInterval, func(time.Time) tea.Msg {
		return debounceTickMsg{tag: tag}
	})
}

// toggleInputFocus は pattern ↔ include の focus を切り替える（proposal #001 D-3）。
// 非アクティブ側は Blur してカーソル/カーソル点滅を消し、
// アクティブ側のみが文字入力を受ける状態にする。
func (g *GrepModel) toggleInputFocus() (Pane, tea.Cmd) {
	if g.focusedInput == grepInputPattern {
		g.textInput.Blur()
		cmd := g.includeInput.Focus()
		g.focusedInput = grepInputInclude
		return g, cmd
	}
	g.includeInput.Blur()
	cmd := g.textInput.Focus()
	g.focusedInput = grepInputPattern
	return g, cmd
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
	// proposal #001 fuzzy 路線: include 非空なら allFiles を fuzzy filter し、
	// マッチしたファイル群を rg に渡す。マッチ 0 件なら rg を呼ばずに空結果へ遷移する
	// (rg --glob と違い ignore を上書きしないので .git/ 等は出ない)。
	// proposal #002 Phase 3: 同時に「パス → MatchedIndexes」マップも保持し、
	// View で各 item のファイルパス部分のハイライトに使う。
	var files []string
	if includeQuery := strings.TrimSpace(g.includeInput.Value()); includeQuery != "" {
		files, g.pathMatchedIndexes = fuzzyFilterFiles(includeQuery, g.allFiles)
		if len(files) == 0 {
			g.searchCancel = nil
			g.items = nil
			g.err = nil
			return g, nil
		}
	} else {
		g.pathMatchedIndexes = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), grepSearchTimeout)
	g.searchCancel = cancel
	g.loading = true
	return g, runGrepCmd(ctx, g.textInput.Value(), files)
}

// SetAllFiles は Finder と共有のファイル列挙結果を取り込む（proposal #001 fuzzy 路線）。
// FilesLoadedMsg を model.go から両ペインに propagate する経路で呼ばれる。
// include 入力中の fuzzy filter ソースとして使う。
func (g *GrepModel) SetAllFiles(files []string) {
	g.allFiles = files
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
	// proposal #002 Phase 3: include 非空時は pathMatchedIndexes が populate されており、
	// 各 item のファイルパス部分にハイライトを挿入する。空時はマップが nil で no-op。
	hasIncludeHighlight := len(g.pathMatchedIndexes) > 0

	var b strings.Builder
	for i, item := range visible {
		if i > 0 {
			b.WriteString("\n")
		}
		absIdx := g.offset + i
		path, _ := parseGrepItem(item)
		// 描画用にサニタイズ。SelectedItem/FilePath/OpenTarget は raw のまま使うため
		// ここで保持される items 自体は変更しない。
		display := sanitize.ForTerminal(path)
		if hasIncludeHighlight {
			if idx, ok := g.pathMatchedIndexes[path]; ok && len(idx) > 0 {
				display = highlightAtIndexes(display, idx)
			}
		}
		display = truncateToWidth(display, itemWidth)
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

// HeaderViews はヘッダー入力欄の View をスライスで返す（HeaderRenderer; #006 / proposal #001 Phase 2）。
// 1 行目: "[Grep]" ラベル + pattern 入力欄。2 行目: "files:" ラベル + include glob 入力欄。
// 2 つのラベル "[Grep]" と "files:" は同じ 7 列幅にすることで `>` 列を縦に整列させる
// （proposal #001 D-4 のレイアウト）。
//
// proposal #001 D-3: focus 中の入力欄ラベルだけを active style (modeLabelStyle) で描画し、
// 非 focus 側は inactiveLabelStyle (dim) にする。Shift+Tab でハイライトが入れ替わるため、
// ラベルだけ見て「いまどちらに文字が流れるか」が判別できる。
func (g *GrepModel) HeaderViews() []string {
	patternLabel := inactiveLabelStyle.Render("[Grep]")
	includeLabel := inactiveLabelStyle.Render("files:")
	if g.focusedInput == grepInputPattern {
		patternLabel = modeLabelStyle.Render("[Grep]")
	} else {
		includeLabel = modeLabelStyle.Render("files:")
	}
	return []string{
		patternLabel + " " + g.textInput.View(),
		includeLabel + " " + g.includeInput.View(),
	}
}

// IncludeValue は include 入力欄の現在値を返す（proposal #001）。
// 値はファイルパスへの fuzzy クエリとして使われ、handleDebounceTick が
// allFiles に対して fuzzyFilterFiles をかけて rg の検索対象を絞る。
func (g *GrepModel) IncludeValue() string { return g.includeInput.Value() }

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
// proposal #001 Phase 2: include 入力欄も同時にクリアし、focus を pattern に戻す
// （"Reset 時の include 欄保持: 残さない" 決定）。
func (g *GrepModel) Reset() {
	if g.searchCancel != nil {
		g.searchCancel()
		g.searchCancel = nil
	}
	g.textInput.SetValue("")
	g.includeInput.SetValue("")
	g.cursor = 0
	g.offset = 0
	g.items = nil
	g.err = nil
	g.pathMatchedIndexes = nil
	g.focusedInput = grepInputPattern
	g.includeInput.Blur()
	g.textInput.Focus()
}

// Focus はテキスト入力にフォーカスを当てる。
// proposal #001 Phase 2: 常に pattern 入力欄に focus を戻す。
// モード切替で再度 Grep に入った際の起点を pattern に固定する。
func (g *GrepModel) Focus() tea.Cmd {
	g.includeInput.Blur()
	g.focusedInput = grepInputPattern
	return g.textInput.Focus()
}

// Blur はテキスト入力のフォーカスを外す。
// proposal #001 Phase 2: pattern と include の両方を blur し、
// モード切替後にカーソル点滅が画面外に残らないようにする。
func (g *GrepModel) Blur() {
	g.textInput.Blur()
	g.includeInput.Blur()
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
