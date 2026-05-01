package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestFinderCursorMovement(t *testing.T) {
	tests := []struct {
		name       string
		items      []string
		keys       []tea.KeyMsg
		wantCursor int
	}{
		{
			name:       "↓でカーソルが1つ下に移動する",
			items:      []string{"a", "b", "c"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown)},
			wantCursor: 1,
		},
		{
			name:       "最下部で↓を押してもカーソルが止まる",
			items:      []string{"a", "b"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown)},
			wantCursor: 1,
		},
		{
			name:       "↑でカーソルが1つ上に移動する",
			items:      []string{"a", "b", "c"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyUp)},
			wantCursor: 1,
		},
		{
			name:       "先頭で↑を押してもカーソルが止まる",
			items:      []string{"a", "b"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyUp)},
			wantCursor: 0,
		},
		{
			name:       "アイテムが空のとき↓を押してもカーソルが0のまま",
			items:      []string{},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown)},
			wantCursor: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFinderModel()
			f.allFiles = tt.items
			f.items = tt.items
			f.loading = false

			var pane Pane = f
			for _, key := range tt.keys {
				pane, _ = pane.Update(key)
			}
			got := pane.(*FinderModel)
			assert.Equal(t, tt.wantCursor, got.cursor)
		})
	}
}

func TestFinderCharacterInput(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"main.go", "go.mod"}
	f.items = f.allFiles
	f.loading = false

	var pane Pane = f
	pane, _ = pane.Update(keyMsg("m"))
	pane, _ = pane.Update(keyMsg("a"))
	pane, _ = pane.Update(keyMsg("i"))

	assert.Equal(t, "mai", pane.Query())
	assert.Equal(t, []string{"main.go"}, pane.(*FinderModel).items)
}

func TestFinderBackspace(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"main.go", "go.mod"}
	f.items = f.allFiles
	f.loading = false

	var pane Pane = f
	pane, _ = pane.Update(keyMsg("m"))
	pane, _ = pane.Update(keyMsg("a"))
	pane, _ = pane.Update(specialKeyMsg(tea.KeyBackspace))

	assert.Equal(t, "m", pane.Query())
}

func TestFinderCursorClampsOnFilter(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"main.go", "go.mod", "README.md"}
	f.items = f.allFiles
	f.cursor = 2
	f.loading = false

	var pane Pane = f
	pane, _ = pane.Update(keyMsg("m"))
	pane, _ = pane.Update(keyMsg("a"))
	pane, _ = pane.Update(keyMsg("i"))

	got := pane.(*FinderModel)
	assert.Equal(t, 0, got.cursor, "フィルタでアイテムが減ったらカーソルがクランプされる")
}

func TestFinderFilesLoadedMsg(t *testing.T) {
	f := NewFinderModel()

	pane, _ := f.Update(FilesLoadedMsg{Items: []string{"a.go", "b.go"}})
	got := pane.(*FinderModel)

	assert.Equal(t, []string{"a.go", "b.go"}, got.allFiles)
	assert.Equal(t, []string{"a.go", "b.go"}, got.items)
	assert.False(t, got.loading)
}

func TestFinderFilesLoadedMsgWithQuery(t *testing.T) {
	f := NewFinderModel()
	f.textInput.SetValue("mai")

	pane, _ := f.Update(FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})
	got := pane.(*FinderModel)

	assert.Equal(t, []string{"main.go"}, got.items)
}

func TestFinderFilesErrorMsg(t *testing.T) {
	f := NewFinderModel()

	pane, _ := f.Update(FilesErrorMsg{Err: assert.AnError})
	got := pane.(*FinderModel)

	assert.False(t, got.loading)
	assert.Equal(t, assert.AnError, got.Err())
}

func TestFinderSelectedItem(t *testing.T) {
	f := NewFinderModel()
	f.items = []string{"main.go", "go.mod"}
	f.cursor = 1

	assert.Equal(t, "go.mod", f.SelectedItem())
	assert.Equal(t, "go.mod", f.FilePath())
}

func TestFinderSelectedItemEmpty(t *testing.T) {
	f := NewFinderModel()
	assert.Equal(t, "", f.SelectedItem())
	assert.Equal(t, "", f.FilePath())
}

func TestFinderView(t *testing.T) {
	f := NewFinderModel()
	f.items = []string{"main.go", "go.mod"}
	f.cursor = 0
	f.loading = false

	view := f.View()
	assert.Contains(t, view, "> main.go")
	assert.Contains(t, view, "  go.mod")
}

// H-2 回帰: ファイル名にエスケープシーケンスが含まれていても、
// View 出力には生の ESC バイトが残らないこと。
// 注: lipgloss 自身も style 付与のため `\x1b[0m` を吐くので、ここでは
// 「入力の生 SGR が連続して残っている」ことを示すペア (例: `[41;97mHIJACKED`) で判定する。
func TestFinderViewSanitizesEscapesInFilenames(t *testing.T) {
	evilName := "name_\x1b[41;97mHIJACKED\x1b[0m_.txt"
	f := NewFinderModel()
	f.items = []string{evilName, "normal.go"}
	f.cursor = 0
	f.loading = false
	f.SetViewSize(10, 80)

	view := f.View()

	assert.NotContains(t, view, "\x1b[41;97m", "raw SGR escape leaked into View")
	assert.NotContains(t, view, "\x1b[41;97mHIJACKED", "raw SGR + payload leaked into View")
	// 可視化された安全表現は残る
	assert.Contains(t, view, `\x1b[41;97m`)
	assert.Contains(t, view, "HIJACKED")
}

// SelectedItem / FilePath / OpenTarget は描画用ではなく
// ファイル読み込み・エディタ起動に使うため、raw のままを返すこと。
func TestFinderRawPathsForOperations(t *testing.T) {
	evilName := "weird_\x1b[31mname\x1b[0m.go"
	f := NewFinderModel()
	f.items = []string{evilName}
	f.cursor = 0

	assert.Equal(t, evilName, f.SelectedItem())
	assert.Equal(t, evilName, f.FilePath())
	gotPath, gotLine := f.OpenTarget()
	assert.Equal(t, evilName, gotPath)
	assert.Equal(t, 0, gotLine)
}

func TestFinderReset(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"a", "b"}
	f.textInput.SetValue("foo")
	f.cursor = 1

	f.Reset()
	assert.Equal(t, "", f.Query())
	assert.Equal(t, 0, f.cursor)
	assert.Equal(t, []string{"a", "b"}, f.items)
}
