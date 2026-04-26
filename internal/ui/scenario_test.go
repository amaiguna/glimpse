package ui

import (
	"strings"
	"testing"

	"github.com/amaiguna/glimpse-tui/internal/grep"
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

	// ローディング中も View() は panic しない
	_ = m.View()

	// ファイルがロードされると一覧に表示される
	m, _ = updateModel(t, m, FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})
	view := m.View()
	assert.Contains(t, stripANSI(view), "main.go")
	assert.Contains(t, stripANSI(view), "go.mod")
	assert.Contains(t, stripANSI(view), "README.md")
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

// #007: rg が broken regex で exit 2 + stderr を返すシナリオ。
// 直前の検索結果が維持されつつ、stderr 内容が読みやすく ESC 込みのレイアウト崩壊なく表示される。
func TestScenario_GrepRecoverableRegexError(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, specialKeyMsg(tea.KeyTab))

	// 直前のヒット結果が表示されている状態を作る
	prevMatches := []grep.Match{
		{File: "main.go", Line: 1, Text: "package main"},
	}
	m, _ = updateModel(t, m, GrepDoneMsg{Matches: prevMatches})
	viewBefore := stripANSI(m.View())
	assert.Contains(t, viewBefore, "main.go", "事前検索結果が表示されている")

	// broken regex によるエラーが入る（#008 の CmdError が伝搬してくる想定）
	cmdErr := &grep.CmdError{
		ExitCode: 2,
		Stderr:   "regex parse error: unclosed character class",
	}
	m, _ = updateModel(t, m, GrepErrorMsg{Err: cmdErr})

	viewAfter := stripANSI(m.View())

	// 1) ステータス行に regex エラー本文が出る（exit status のノイズなし）
	assert.Contains(t, viewAfter, "regex parse error: unclosed character class")
	assert.NotContains(t, viewAfter, "exit status 2",
		"exit status の冗長表記は UI に出さない")

	// 2) 通常レイアウトが維持されている (textinput / 枠線)
	assert.Contains(t, viewAfter, "[Grep]")
	assert.Contains(t, viewAfter, "┌")
	assert.Contains(t, viewAfter, "└")

	// 3) 直前の検索結果（main.go）が消えていない
	assert.Contains(t, viewAfter, "main.go",
		"broken regex 入力中も前回ヒットは維持されるべき")
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
