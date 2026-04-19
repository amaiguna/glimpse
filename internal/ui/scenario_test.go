package ui

import (
	"strings"
	"testing"

	"github.com/amaiguna/telescope-tui/internal/grep"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateModel は tea.Model を更新し、Model として返すヘルパー。
// 内部フィールドに触れず、Msg → Update → (Model, Cmd) の流れだけでテストする。
func updateModel(t *testing.T, m tea.Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	result, cmd := m.Update(msg)
	got, ok := result.(Model)
	require.True(t, ok, "Update が Model を返すこと")
	return got, cmd
}

// typeString は文字列を1文字ずつ KeyMsg として送るヘルパー。
func typeString(t *testing.T, m tea.Model, s string) Model {
	t.Helper()
	cur := m
	for _, r := range s {
		cur, _ = updateModel(t, cur, keyMsg(string(r)))
	}
	return cur.(Model)
}

// --- Finder シナリオ ---

func TestScenario_FinderLoadAndDisplay(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// ローディング中は "loading" が表示される
	view := m.View()
	assert.Contains(t, view, "loading")

	// ファイルがロードされると一覧に表示される
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})
	view = m.View()
	assert.Contains(t, stripANSI(view), "main.go")
	assert.Contains(t, stripANSI(view), "go.mod")
	assert.Contains(t, stripANSI(view), "README.md")
	assert.NotContains(t, view, "loading")
}

func TestScenario_FinderFilterByTyping(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})

	// "mai" と入力すると main.go だけに絞られる
	m = typeString(t, m, "mai")
	view := stripANSI(m.View())
	assert.Contains(t, view, "main.go")
	assert.NotContains(t, view, "go.mod")
	assert.NotContains(t, view, "README.md")
}

func TestScenario_FinderCursorAndSelect(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})

	// 初期状態: 1つ目にカーソル → ">" が main.go の行にある
	view := stripANSI(m.View())
	assert.Contains(t, view, "> main.go")

	// ↓ で2つ目に移動
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyDown))
	view = stripANSI(m.View())
	assert.Contains(t, view, "> go.mod")

	// Enter でコマンドが返る
	_, cmd := updateModel(t, m, specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Enter で Cmd が返される")
}

func TestScenario_FinderCtrlPCtrlN(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"a.go", "b.go", "c.go"}})

	// Ctrl+N で下移動
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyCtrlN))
	view := stripANSI(m.View())
	assert.Contains(t, view, "> b.go")

	// Ctrl+P で上移動
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyCtrlP))
	view = stripANSI(m.View())
	assert.Contains(t, view, "> a.go")
}

func TestScenario_FinderEmptyAfterFilter(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod"}})

	// マッチしないクエリを入力
	m = typeString(t, m, "zzz")

	// Enter しても Cmd は返らない（選択アイテムがない）
	_, cmd := updateModel(t, m, specialKeyMsg(tea.KeyEnter))
	assert.Nil(t, cmd)
}

func TestScenario_FinderError(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesErrorMsg{Err: assert.AnError})

	view := m.View()
	assert.Contains(t, view, "error")
}

// --- Grep シナリオ ---

func TestScenario_GrepSwitchAndBack(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod"}})

	// Tab で Grep モードに切り替え
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))
	view := m.View()
	assert.Contains(t, view, "Grep")
	assert.NotContains(t, view, "Files")

	// Tab で Finder モードに戻る
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))
	view = m.View()
	assert.Contains(t, view, "Files")
	// Finder に戻ったらファイル一覧が復元されている
	assert.Contains(t, stripANSI(view), "main.go")
}

func TestScenario_GrepReceiveResults(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Grep モードへ
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))

	// 検索結果を受信
	m, _ = updateModel(t, m, GrepDoneMsg{Matches: []grep.Match{
		{File: "main.go", Line: 5, Text: "func main() {"},
		{File: "util.go", Line: 10, Text: "func helper() {"},
	}})

	view := stripANSI(m.View())
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "util.go")
}

func TestScenario_GrepCursorAndSelect(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))
	m, _ = updateModel(t, m, GrepDoneMsg{Matches: []grep.Match{
		{File: "main.go", Line: 5, Text: "func main() {"},
		{File: "util.go", Line: 10, Text: "func helper() {"},
	}})

	// ↓ で2つ目に移動
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyDown))
	view := stripANSI(m.View())
	assert.Contains(t, view, "> util.go")

	// Enter でコマンドが返る
	_, cmd := updateModel(t, m, specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Grep モードの Enter で Cmd が返される")
}

func TestScenario_GrepError(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))
	m, _ = updateModel(t, m, GrepErrorMsg{Err: assert.AnError})

	view := m.View()
	assert.Contains(t, view, "error")
}

// --- モード切替エッジケース ---

func TestScenario_SwitchModeAfterScroll(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"a", "b", "c", "d", "e"}})

	// カーソルを最後まで移動
	for i := 0; i < 4; i++ {
		m, _ = updateModel(t, m, specialKeyMsg(tea.KeyDown))
	}

	// Tab で Grep に切り替え → panic しない
	assert.NotPanics(t, func() {
		m2, _ := updateModel(t, m, specialKeyMsg(tea.KeyTab))
		_ = m2.View()
	})
}

func TestScenario_SwitchModeResetsQuery(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod"}})

	// Finder で "main" と入力
	m = typeString(t, m, "main")
	view := stripANSI(m.View())
	assert.Contains(t, view, "main")

	// Grep に切り替えてから Finder に戻る
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))

	// クエリがリセットされ、全ファイルが見える
	view = stripANSI(m.View())
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "go.mod")
}

// --- 終了キー ---

func TestScenario_QuitKeys(t *testing.T) {
	quitKeys := []tea.KeyMsg{
		{Type: tea.KeyCtrlC},
		specialKeyMsg(tea.KeyEscape),
	}
	for _, key := range quitKeys {
		m := NewModel()
		_, cmd := updateModel(t, m, key)
		assert.NotNil(t, cmd, "終了キーで Cmd が返される")
	}
}

// --- ウィンドウリサイズ ---

func TestScenario_ResizeDoesNotBreakView(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod"}})

	// リサイズしても View() が壊れない
	sizes := []tea.WindowSizeMsg{
		{Width: 40, Height: 12},
		{Width: 160, Height: 50},
		{Width: 20, Height: 8},
	}
	for _, size := range sizes {
		m, _ = updateModel(t, m, size)
		view := m.View()
		for i, line := range strings.Split(view, "\n") {
			w := len([]rune(stripANSI(line)))
			assert.LessOrEqual(t, w, size.Width,
				"リサイズ %dx%d: 行 %d が幅を超えている", size.Width, size.Height, i)
		}
	}
}
