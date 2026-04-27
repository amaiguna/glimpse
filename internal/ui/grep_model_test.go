package ui

import (
	"errors"
	"testing"

	"github.com/amaiguna/glimpse-tui/internal/grep"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// #007: rg が exit code 2 + stderr で返したケース（典型: regex parse error）は、
// 表示上の冗長な "exit status 2:" プレフィックスを除いて stderr 本文のみを surface する。
func TestGrepErrorMsgWithCmdErrorShowsStderrOnly(t *testing.T) {
	g := NewGrepModel()
	g.loading = true

	cmdErr := &grep.CmdError{
		ExitCode: 2,
		Stderr:   "regex parse error: unclosed character class",
		Err:      errors.New("exit status 2"),
	}
	pane, _ := g.Update(GrepErrorMsg{Err: cmdErr})
	got := pane.(*GrepModel)

	assert.False(t, got.loading)
	require.NotNil(t, got.Err())
	msg := got.Err().Error()
	assert.Contains(t, msg, "regex parse error: unclosed character class")
	assert.NotContains(t, msg, "exit status 2",
		"UI 表示時には exit status の冗長表記は除く（stderr 本文だけにする）")
}

// #007: 前回の検索結果は GrepErrorMsg 受信後も維持され、
// ユーザーが broken regex を修正している間にリストが空に戻らない。
func TestGrepKeepsItemsOnError(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"a.go:1:foo", "b.go:2:bar"}

	cmdErr := &grep.CmdError{
		ExitCode: 2,
		Stderr:   "regex parse error: unclosed character class",
		Err:      errors.New("exit status 2"),
	}
	pane, _ := g.Update(GrepErrorMsg{Err: cmdErr})
	got := pane.(*GrepModel)

	assert.Equal(t, []string{"a.go:1:foo", "b.go:2:bar"}, got.items,
		"エラー時も前回のヒットリストは維持される必要がある")
}

// #007 取りこぼし: regex エラー → クエリを空に戻したとき、items だけでなく err も消える。
// debounceTick で「これから検索しない（idle に入る）」ことが確定したタイミングで
// 古い regex エラーは陳腐化するため、ここで一緒にクリアする。
func TestGrepClearsErrorWhenQueryBecomesEmpty(t *testing.T) {
	g := NewGrepModel()
	g.err = errors.New("regex parse error: unclosed character class")
	g.items = []string{"a.go:1:foo"}
	g.textInput.SetValue("")
	g.debounceTag = 1

	pane, _ := g.Update(debounceTickMsg{tag: 1})
	got := pane.(*GrepModel)

	assert.Nil(t, got.Err(), "クエリが空になったらエラー表示も消えるべき")
	assert.Nil(t, got.items, "items も既存通り消える")
}

// #007: stderr が空の CmdError（または他のエラー型）は元のメッセージを尊重する。
func TestGrepErrorMsgWithoutStderrFallsBackToOriginalError(t *testing.T) {
	g := NewGrepModel()

	pane, _ := g.Update(GrepErrorMsg{Err: errors.New("rg: executable not found in $PATH")})
	got := pane.(*GrepModel)

	require.NotNil(t, got.Err())
	assert.Equal(t, "rg: executable not found in $PATH", got.Err().Error())
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

// M-3 回帰: 新しい debounceTick が発火したら前回の検索 context を cancel して
// 古い rg プロセスを kill できること（stdout 溜め込み防止）。
// Reset でも cancel が呼ばれる（モード切替時に進行中の rg を掃除する）。
func TestGrepCancelsPreviousSearchOnNewDebounce(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("foo")
	g.debounceTag = 1

	called := 0
	g.searchCancel = func() { called++ }

	_, cmd := g.Update(debounceTickMsg{tag: 1})
	assert.Equal(t, 1, called, "新規 debounce で前回 cancel が呼ばれる")
	assert.NotNil(t, cmd)
	assert.NotNil(t, g.searchCancel, "新しい cancel がセットされている")
}

func TestGrepCancelsSearchOnReset(t *testing.T) {
	g := NewGrepModel()
	called := false
	g.searchCancel = func() { called = true }

	g.Reset()
	assert.True(t, called, "Reset 時に進行中の検索は cancel される")
	assert.Nil(t, g.searchCancel)
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
