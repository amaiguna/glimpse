package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proposal #001 Phase 2: Grep モードに include glob 入力欄を追加する。
// このファイルは UI 状態（focus 管理 / 入力欄数 / placeholder / Reset 挙動）に関する
// テストをまとめる。Phase 3 で rg --glob に配線するときの足場。

// HeaderViews は pattern と include の 2 入力欄を返す。
// 1 行目は "[Grep]" ラベル + pattern、2 行目は "files:" ラベル + include。
// 親 Model 側はこの 2 行を縦に並べて描画する。
func TestGrepHeaderViewsReturnsPatternAndIncludeLines(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("foo")

	views := g.HeaderViews()
	require.Len(t, views, 2, "Phase 2 で Grep は 2 入力欄に拡張される")
	assert.Contains(t, views[0], "[Grep]", "1 行目にモードラベル [Grep]")
	assert.Contains(t, views[0], g.textInput.View(), "1 行目に pattern 入力欄の View")
	assert.Contains(t, views[1], "files:", "2 行目は include 用ラベルから始まる（discoverable）")
	assert.Contains(t, views[1], g.includeInput.View(), "2 行目に include 入力欄の View が含まれる")
}

// 初期状態では pattern にフォーカス、include は blur されている。
// モード切替直後の Tab キー操作・テキスト入力が pattern に流れる前提。
func TestGrepInitialFocusIsPattern(t *testing.T) {
	g := NewGrepModel()
	assert.True(t, g.textInput.Focused(), "起動直後は pattern にフォーカス")
	assert.False(t, g.includeInput.Focused(), "include は初期 blur")
}

// Shift+Tab で pattern ↔ include の focus を行き来する（D-3）。
// 一度押すと include に移動、もう一度で pattern に戻る。
func TestGrepShiftTabTogglesInputFocus(t *testing.T) {
	g := NewGrepModel()

	pane, _ := g.Update(specialKeyMsg(tea.KeyShiftTab))
	got := pane.(*GrepModel)
	assert.False(t, got.textInput.Focused(), "Shift+Tab 1 回目: pattern blur")
	assert.True(t, got.includeInput.Focused(), "Shift+Tab 1 回目: include focus")

	pane, _ = got.Update(specialKeyMsg(tea.KeyShiftTab))
	got = pane.(*GrepModel)
	assert.True(t, got.textInput.Focused(), "Shift+Tab 2 回目: pattern に戻る")
	assert.False(t, got.includeInput.Focused(), "Shift+Tab 2 回目: include blur")
}

// include にフォーカスを当てて文字を入力すると include の値だけが変わる。
// pattern 側は不変。
func TestGrepIncludeInputAcceptsKeystrokesWhenFocused(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("hello")

	// include に focus
	pane, _ := g.Update(specialKeyMsg(tea.KeyShiftTab))
	got := pane.(*GrepModel)

	pane, _ = got.Update(keyMsg("*"))
	pane, _ = pane.(*GrepModel).Update(keyMsg("."))
	pane, _ = pane.(*GrepModel).Update(keyMsg("g"))
	pane, _ = pane.(*GrepModel).Update(keyMsg("o"))
	got = pane.(*GrepModel)

	assert.Equal(t, "*.go", got.IncludeValue())
	assert.Equal(t, "hello", got.Query(), "pattern 側は影響を受けない")
}

// Phase 2 では include への入力は rg を発火させない (UI 状態保持のみ)。
// Phase 3 で配線するときに、ここのアサーションを反転させる予定。
// 注: textinput は cursor blink 用の Cmd を常に返す（Update の副作用）ため、
// "Cmd が nil" は契約に含めない。検索発火の代理として debounceTag を見る。
func TestGrepIncludeInputDoesNotTriggerDebounce(t *testing.T) {
	g := NewGrepModel()
	g.debounceTag = 0

	// include に focus
	g.Update(specialKeyMsg(tea.KeyShiftTab))

	prevTag := g.debounceTag
	g.Update(keyMsg("*"))

	assert.Equal(t, prevTag, g.debounceTag,
		"Phase 2 では include への入力で debounceTag は進まない (rg 発火しない)")
	assert.Equal(t, "*", g.IncludeValue(), "include への入力は値に反映される")
}

// pattern 側の入力は従来通り debounce → rg 発火経路に乗る。
// include 追加で既存の Grep 検索体験が壊れていないことを pin する。
func TestGrepPatternInputStillTriggersDebounce(t *testing.T) {
	g := NewGrepModel()
	prevTag := g.debounceTag

	_, cmd := g.Update(keyMsg("f"))

	assert.Greater(t, g.debounceTag, prevTag, "pattern 入力では debounceTag が進む")
	assert.NotNil(t, cmd, "pattern 入力では debounce Cmd が返る")
}

// Reset は pattern と include の両方をクリアし、focus を pattern に戻す。
// proposal の "Reset 時の include 欄保持: 残さない" 決定に対応。
func TestGrepResetClearsBothInputsAndRestoresPatternFocus(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("foo")
	g.includeInput.SetValue("*.go")
	// include に focus を移しておく
	g.Update(specialKeyMsg(tea.KeyShiftTab))

	g.Reset()

	assert.Equal(t, "", g.Query(), "pattern クリア")
	assert.Equal(t, "", g.IncludeValue(), "include クリア")
	assert.True(t, g.textInput.Focused(), "Reset 後は pattern にフォーカスが戻る")
	assert.False(t, g.includeInput.Focused())
}

// include 入力欄の placeholder は何が入るか（ユーザに glob の書式例を示す）の
// discover を担う。空時に grayed-out 表示されることが proposal D-4 の前提。
func TestGrepIncludeInputHasPlaceholder(t *testing.T) {
	g := NewGrepModel()
	require.NotEmpty(t, g.includeInput.Placeholder,
		"include 入力欄には書式例の placeholder が必要 (discoverability)")
	// 具体的な例文は proposal の "e.g. *.go !testdata/**" に従う。
	assert.True(t,
		strings.Contains(g.includeInput.Placeholder, "*.go") ||
			strings.Contains(g.includeInput.Placeholder, "glob"),
		"placeholder に glob 例が含まれる: %q", g.includeInput.Placeholder)
}

// Blur は両方の入力欄を blur する。モード切替時にカーソル表示が完全に消えるため。
func TestGrepBlurClearsBothInputFocus(t *testing.T) {
	g := NewGrepModel()
	g.includeInput.Focus()
	g.textInput.Focus() // 念のため両方 focus 状態に

	g.Blur()
	assert.False(t, g.textInput.Focused())
	assert.False(t, g.includeInput.Focused())
}

// Focus は pattern にフォーカスを戻す。モード切替で再度 Grep に入った直後の挙動。
func TestGrepFocusReturnsToPatternInput(t *testing.T) {
	g := NewGrepModel()
	g.Blur()
	g.includeInput.Focus()

	g.Focus()
	assert.True(t, g.textInput.Focused(), "Focus() は常に pattern を focus する")
	assert.False(t, g.includeInput.Focused())
}

// proposal #001 D-3 補強: focus 中の入力欄ラベルだけ強調表示し、
// もう一方は dim で表示する。Shift+Tab でハイライトが入れ替わることで
// 「今どっちの入力欄に文字が流れるか」がラベルだけ見て判別できる。
//
// テストはスタイル文字列の包含で判定する: HeaderViews の戻り値に
// modeLabelStyle 由来の ANSI が active 行のラベル位置だけに現れることを
// 確認する。inactive 側は inactiveLabelStyle が適用される。
func TestGrepHeaderLabelHighlightFollowsFocus(t *testing.T) {
	g := NewGrepModel()

	// 初期: pattern focus → [Grep] が active、files: が inactive
	views := g.HeaderViews()
	require.Len(t, views, 2)
	assert.Contains(t, views[0], modeLabelStyle.Render("[Grep]"),
		"pattern focus 時は [Grep] が active style で描画される")
	assert.Contains(t, views[1], inactiveLabelStyle.Render("files:"),
		"pattern focus 時は files: は inactive (dim)")
	assert.NotContains(t, views[1], modeLabelStyle.Render("files:"),
		"pattern focus 時に files: が active style になっていてはいけない")

	// Shift+Tab → include focus に切り替わる → ハイライトも入れ替わる
	pane, _ := g.Update(specialKeyMsg(tea.KeyShiftTab))
	got := pane.(*GrepModel)
	views = got.HeaderViews()
	assert.Contains(t, views[1], modeLabelStyle.Render("files:"),
		"include focus 時は files: が active style")
	assert.Contains(t, views[0], inactiveLabelStyle.Render("[Grep]"),
		"include focus 時は [Grep] は inactive (dim)")
	assert.NotContains(t, views[0], modeLabelStyle.Render("[Grep]"),
		"include focus 時に [Grep] が active のままでいてはいけない")
}

// Finder は単一入力欄なので [Files] ラベルは常に active 表示。
// Pane が自身でラベルを完成形で返す責任を持つことの compile-time/runtime 検証。
func TestFinderHeaderLabelAlwaysActive(t *testing.T) {
	f := NewFinderModel()
	views := f.HeaderViews()
	require.Len(t, views, 1)
	assert.Contains(t, views[0], modeLabelStyle.Render("[Files]"),
		"Finder の [Files] ラベルは常に active style で描画される")
}
