package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaiguna/telescope-tui/internal/grep"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyMsg はテスト用にキーメ���セージを生成するヘルパー。
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// specialKeyMsg は特殊キーのメッセージを生成するヘルパー。
func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

// stripANSI は ANSI エスケープシーケンスを除去するテスト用ヘルパー。
func stripANSI(s string) string {
	result := strings.Builder{}
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// --- 親 Model: モード切替テスト ---

func TestModeSwitchWithTab(t *testing.T) {
	m := NewModel()
	m.finderPane.allFiles = []string{"a", "b"}
	m.finderPane.items = []string{"a", "b"}

	// Finder → Grep
	model, _ := m.Update(specialKeyMsg(tea.KeyTab))
	got := model.(Model)
	assert.Equal(t, ModeGrep, got.mode)

	// Grep → Finder (allFiles が復元される)
	model, _ = got.Update(specialKeyMsg(tea.KeyTab))
	got = model.(Model)
	assert.Equal(t, ModeFinder, got.mode)
	assert.Equal(t, []string{"a", "b"}, got.finderPane.items)
}

func TestModeSwitchAfterScrollDoesNotPanic(t *testing.T) {
	m := NewModel()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m3, _ := m2.Update(FilesLoadedMsg{Items: []string{"a", "b", "c", "d", "e"}})

	// カーソルを下に移動
	cur := m3
	for i := 0; i < 4; i++ {
		cur, _ = cur.Update(specialKeyMsg(tea.KeyDown))
	}

	// Tab で Grep に切り替え — panic しないこと
	assert.NotPanics(t, func() {
		model, _ := cur.Update(specialKeyMsg(tea.KeyTab))
		got := model.(Model)
		// View() も panic しないこと
		_ = got.View()
	})
}

// --- 親 Model: グローバルキーテスト ---

func TestQuitKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "Ctrl+C で終了", key: tea.KeyMsg{Type: tea.KeyCtrlC}},
		{name: "Esc で終了", key: specialKeyMsg(tea.KeyEscape)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			_, cmd := m.Update(tt.key)
			assert.NotNil(t, cmd, "終了コマンド���返される")
		})
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := model.(Model)
	assert.Equal(t, 120, got.width)
	assert.Equal(t, 40, got.height)
}

func TestInitReturnsCmd(t *testing.T) {
	m := NewModel()
	cmd := m.Init()
	assert.NotNil(t, cmd, "Init はファイル列挙コマンドを返す")
}

// --- 親 Model: Enter キーテスト ---

func TestEnterInFinderMode(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod"}
	m.finderPane.cursor = 1

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Enter でエディタ起動コマンドが返される")
}

func TestEnterInGrepMode(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.items = []string{"main.go:10:func main()"}
	m.grepPane.cursor = 0

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Grep モードの Enter でエディタ起動コマンドが返される")
}

func TestEnterWithNoItems(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.Nil(t, cmd, "アイテムがないとき Enter は何もしない")
}

// --- 親 Model: Msg 委譲テスト ---

func TestFilesLoadedMsgDelegatesToFinder(t *testing.T) {
	m := NewModel()

	model, _ := m.Update(FilesLoadedMsg{Items: []string{"a.go", "b.go"}})
	got := model.(Model)

	assert.Equal(t, []string{"a.go", "b.go"}, got.finderPane.allFiles)
	assert.Equal(t, []string{"a.go", "b.go"}, got.finderPane.items)
	assert.False(t, got.finderPane.loading)
}

func TestGrepDoneMsgDelegatesToGrep(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep

	model, _ := m.Update(GrepDoneMsg{Matches: []grep.Match{
		{File: "a.go", Line: 1, Text: "foo"},
	}})
	got := model.(Model)

	assert.Equal(t, []string{"a.go:1:foo"}, got.grepPane.items)
}

func TestCharacterInputDelegatesToActivePane(t *testing.T) {
	m := NewModel()
	m.finderPane.allFiles = []string{"main.go", "go.mod"}
	m.finderPane.items = m.finderPane.allFiles

	model, _ := m.Update(keyMsg("m"))
	got := model.(Model)

	assert.Equal(t, "m", got.finderPane.Query())
}

func TestCursorMoveDelegatesToActivePane(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"a", "b", "c"}

	model, _ := m.Update(specialKeyMsg(tea.KeyDown))
	got := model.(Model)

	assert.Equal(t, 1, got.finderPane.cursor)
}

// --- 親 Model: プレビュー統合テスト ---

// drainBatchCmds は tea.Batch が返す Cmd を全て実行し、
// PreviewLoadedMsg があれば Model に適用するヘルパー。
func drainCmds(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	// tea.Batch は BatchMsg ([]Cmd) を返す
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainCmds(t, m, c)
		}
		return m
	}
	if _, ok := msg.(PreviewLoadedMsg); ok {
		result, _ := m.Update(msg)
		return result.(Model)
	}
	return m
}

func TestPreviewUpdatesOnCursorMove(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	require.NoError(t, os.WriteFile(fileA, []byte("package a\n"), 0644))
	require.NoError(t, os.WriteFile(fileB, []byte("package b\n"), 0644))

	m := NewModel()
	m.finderPane.items = []string{fileA, fileB}
	m.finderPane.allFiles = []string{fileA, fileB}
	m.finderPane.loading = false

	model, cmd := m.Update(specialKeyMsg(tea.KeyDown))
	got := drainCmds(t, model.(Model), cmd)
	assert.Contains(t, stripANSI(got.previewContent), "package b")
}

func TestPreviewClearsWhenNoItems(t *testing.T) {
	m := NewModel()
	m.previewContent = "stale"
	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)
	assert.Equal(t, "", got.previewContent)
}

func TestPreviewInGrepMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(file, []byte("package main\n"), 0644))

	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.items = []string{file + ":1:package main"}
	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.Contains(t, stripANSI(got.previewContent), "package main")
}

func TestGrepPreviewRange(t *testing.T) {
	tests := []struct {
		name          string
		item          string
		visibleHeight int
		want          int
	}{
		{"中央配置", "main.go:50:text", 20, 40},
		{"先頭クランプ", "main.go:3:text", 20, 1},
		{"1行目", "main.go:1:text", 20, 1},
		{"アイテムなし", "", 20, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGrepModel()
			if tt.item != "" {
				g.items = []string{tt.item}
			}
			got := g.PreviewRange(tt.visibleHeight)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGrepDecoratePreviewTargetsCorrectLine(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:2:func main()"}

	// PreviewRange を呼んで previewStartLine をセット
	g.PreviewRange(20)

	content := "package main\nfunc main() {\n}\n"
	result := g.DecoratePreview(content, 40)

	lines := strings.Split(result, "\n")
	// stripANSI しても元テキストが残っていること（行が壊れていない）
	assert.Equal(t, "package main", stripANSI(lines[0]))
	assert.Equal(t, "func main() {", stripANSI(lines[1]))
	assert.Equal(t, "}", stripANSI(lines[2]))
}

func TestGrepDecoratePreviewWithOffset(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:50:target line"}

	// visibleHeight=20 → startLine=40
	startLine := g.PreviewRange(20)
	assert.Equal(t, 40, startLine)

	// プレビューは40行目から表示されている想定
	// 50行目は表示上のインデックス10 (50-40=10)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", startLine+i)
	}
	content := strings.Join(lines, "\n")
	result := g.DecoratePreview(content, 80)

	resultLines := strings.Split(result, "\n")
	// 10番目の行 (0-indexed) がハイライト対象（stripANSI で元テキストが残る）
	assert.Equal(t, "line 50", stripANSI(resultLines[10]))
	// ハイライト対象外の行は変更なし
	assert.Equal(t, "line 49", resultLines[9])
}

func TestGrepDecoratePreviewEmptyContent(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:1:package main"}
	g.PreviewRange(20)
	assert.Equal(t, "", g.DecoratePreview("", 40))
}

func TestGrepDecoratePreviewNoItems(t *testing.T) {
	g := NewGrepModel()
	content := "package main\n"
	assert.Equal(t, content, g.DecoratePreview(content, 40))
}

func TestFinderDecoratePreviewPassthrough(t *testing.T) {
	f := NewFinderModel()
	content := "package main\nfunc main() {\n}\n"
	result := f.DecoratePreview(content, 40)
	assert.Equal(t, content, result, "Finder モードではプレビューを加工しない")
}

// --- View テスト ---

func TestViewContainsQuery(t *testing.T) {
	m := NewModel()
	m.finderPane.textInput.SetValue("main")
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "main")
}

func TestViewContainsItems(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod"}
	m.finderPane.loading = false
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "go.mod")
}

func TestViewShowsCursorIndicator(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod"}
	m.finderPane.cursor = 1
	m.finderPane.loading = false
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "> go.mod")
}

func TestViewShowsModeLabel(t *testing.T) {
	tests := []struct {
		name  string
		mode  Mode
		label string
	}{
		{name: "Finder モードのラベル", mode: ModeFinder, label: "Files"},
		{name: "Grep モードのラベル", mode: ModeGrep, label: "Grep"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.mode = tt.mode
			m.finderPane.loading = false
			m.width = 80
			m.height = 24

			view := m.View()
			assert.Contains(t, view, tt.label)
		})
	}
}

func TestViewShowsError(t *testing.T) {
	m := NewModel()
	m.finderPane.err = errors.New("something went wrong")
	m.finderPane.loading = false
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "something went wrong")
}

func TestViewShowsLoading(t *testing.T) {
	m := NewModel()
	m.finderPane.loading = true
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "loading")
}

func TestViewContainsPreview(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go"}
	m.finderPane.loading = false
	m.previewContent = "package main\nfunc main() {}\n"
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "package main")
}

func TestViewPreviewLineTruncatedToFitWidth(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"a.txt"}
	m.finderPane.loading = false
	m.previewContent = strings.Repeat("x", 200) + "\n"
	m.width = 60
	m.height = 10

	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, m.width,
			"行がターミナル幅を超えている（visual width=%d）", w)
	}
}

func TestViewFillsFullHeight(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"a.txt"}
	m.finderPane.loading = false
	m.previewContent = "short\n"
	m.width = 80
	m.height = 20

	view := m.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	assert.GreaterOrEqual(t, len(lines), m.height-1,
		"View() がタ��ミナル高さ分の行を出力していない")
}

func TestViewPanesDoNotOverlap(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod", "README.md"}
	m.finderPane.loading = false
	m.previewContent = "\x1b[38;5;197mpackage\x1b[0m \x1b[38;5;148mmain\x1b[0m\n" +
		strings.Repeat("\x1b[38;5;231m"+strings.Repeat("x", 100)+"\x1b[0m\n", 20)
	m.width = 80
	m.height = 24

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, m.width,
			"行 %d がターミナル幅を超えている（visual width=%d）", i, w)
	}
}

// --- parseGrepItem テスト ---

func TestParseGrepItem(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFile string
		wantLine int
	}{
		{name: "正常なgrep形式", input: "main.go:10:func main()", wantFile: "main.go", wantLine: 10},
		{name: "コロンなしの文字列", input: "main.go", wantFile: "main.go", wantLine: 0},
		{name: "行番号が非数値", input: "main.go:abc:func main()", wantFile: "main.go:abc:func main()", wantLine: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, line := parseGrepItem(tt.input)
			assert.Equal(t, tt.wantFile, file)
			assert.Equal(t, tt.wantLine, line)
		})
	}
}
