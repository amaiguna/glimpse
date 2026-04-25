package ui

import (
	"testing"

	"github.com/amaiguna/glimpse-tui/internal/grep"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestGrepCursorMovement(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"a.go:1:foo", "b.go:2:bar", "c.go:3:baz"}

	var pane Pane = g
	pane, _ = pane.Update(specialKeyMsg(tea.KeyDown))
	assert.Equal(t, 1, pane.(*GrepModel).cursor)

	pane, _ = pane.Update(specialKeyMsg(tea.KeyUp))
	assert.Equal(t, 0, pane.(*GrepModel).cursor)
}

func TestGrepCharacterInputReturnsDebounceCmd(t *testing.T) {
	g := NewGrepModel()

	_, cmd := g.Update(keyMsg("f"))
	assert.NotNil(t, cmd, "Grep モードの文字入力でデバウンス Cmd が返される")
	assert.Equal(t, "f", g.Query())
}

func TestGrepBackspace(t *testing.T) {
	g := NewGrepModel()

	g.Update(keyMsg("f"))
	g.Update(keyMsg("o"))
	g.Update(specialKeyMsg(tea.KeyBackspace))

	assert.Equal(t, "f", g.Query())
}

func TestGrepDoneMsgUpdatesItems(t *testing.T) {
	g := NewGrepModel()
	g.loading = true

	matches := []grep.Match{
		{File: "main.go", Line: 10, Text: "func main()"},
		{File: "util.go", Line: 5, Text: "func helper()"},
	}

	pane, _ := g.Update(GrepDoneMsg{Matches: matches})
	got := pane.(*GrepModel)

	assert.Equal(t, []string{"main.go:10:func main()", "util.go:5:func helper()"}, got.items)
	assert.False(t, got.loading)
}

func TestGrepDoneMsgEmpty(t *testing.T) {
	g := NewGrepModel()
	g.loading = true

	pane, _ := g.Update(GrepDoneMsg{Matches: nil})
	got := pane.(*GrepModel)

	assert.Nil(t, got.items)
	assert.False(t, got.loading)
}

func TestGrepErrorMsg(t *testing.T) {
	g := NewGrepModel()
	g.loading = true

	pane, _ := g.Update(GrepErrorMsg{Err: assert.AnError})
	got := pane.(*GrepModel)

	assert.False(t, got.loading)
	assert.Equal(t, assert.AnError, got.Err())
}

func TestGrepDebounceTickMsg(t *testing.T) {
	t.Run("タグ一致でCmdが返される", func(t *testing.T) {
		g := NewGrepModel()
		g.textInput.SetValue("foo")
		g.debounceTag = 5

		_, cmd := g.Update(debounceTickMsg{tag: 5})
		assert.NotNil(t, cmd)
		assert.True(t, g.loading)
	})

	t.Run("タグ不一致で無視される", func(t *testing.T) {
		g := NewGrepModel()
		g.textInput.SetValue("foo")
		g.debounceTag = 5

		_, cmd := g.Update(debounceTickMsg{tag: 3})
		assert.Nil(t, cmd)
	})

	t.Run("クエリ空ならCmdなし", func(t *testing.T) {
		g := NewGrepModel()
		g.textInput.SetValue("")
		g.debounceTag = 5

		_, cmd := g.Update(debounceTickMsg{tag: 5})
		assert.Nil(t, cmd)
		assert.Nil(t, g.items)
	})
}

func TestGrepSelectedItem(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:10:func main()", "util.go:5:func helper()"}
	g.cursor = 0

	assert.Equal(t, "main.go:10:func main()", g.SelectedItem())
	assert.Equal(t, "main.go", g.FilePath())
}

func TestGrepSelectedItemEmpty(t *testing.T) {
	g := NewGrepModel()
	assert.Equal(t, "", g.SelectedItem())
	assert.Equal(t, "", g.FilePath())
}

func TestGrepView(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:10:func main()", "util.go:5:func helper()"}
	g.cursor = 1

	view := g.View()
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "util.go")
	assert.NotContains(t, view, "func main()")
	assert.NotContains(t, view, "func helper()")
}

// H-3 回帰: grep 結果のファイル名にエスケープシーケンスが含まれていても、
// View 出力には raw な ESC バイトの SGR 注入が残らないこと。
func TestGrepViewSanitizesEscapesInFilenames(t *testing.T) {
	evilItem := "name_\x1b[41;97mHIJACKED\x1b[0m_.go:42:matched text"
	g := NewGrepModel()
	g.items = []string{evilItem}
	g.cursor = 0
	g.SetViewSize(10, 80)

	view := g.View()

	assert.NotContains(t, view, "\x1b[41;97m", "raw SGR escape leaked into View")
	assert.NotContains(t, view, "\x1b[0m", "raw SGR reset leaked into View")
	assert.Contains(t, view, `\x1b[41;97m`, "サニタイズ済みの可視表現が描画される")
	assert.Contains(t, view, "HIJACKED")
}

// SelectedItem / FilePath / OpenTarget は描画用ではなくファイル読み込み・
// エディタ起動に使うため、raw のままを返すこと。
// 特に OpenTarget はパスと行番号を分離して返すので、ESC 含むパスでも parseGrepItem が
// 正しく動作することを確認する。
func TestGrepRawPathsForOperations(t *testing.T) {
	evilName := "weird_\x1b[31mname\x1b[0m.go"
	evilItem := evilName + ":42:matched"
	g := NewGrepModel()
	g.items = []string{evilItem}
	g.cursor = 0

	assert.Equal(t, evilItem, g.SelectedItem())
	assert.Equal(t, evilName, g.FilePath())
	gotPath, gotLine := g.OpenTarget()
	assert.Equal(t, evilName, gotPath)
	assert.Equal(t, 42, gotLine)
}

func TestGrepReset(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("foo")
	g.cursor = 3
	g.items = []string{"a", "b", "c", "d"}

	g.Reset()
	assert.Equal(t, "", g.Query())
	assert.Equal(t, 0, g.cursor)
	assert.Nil(t, g.items)
}
