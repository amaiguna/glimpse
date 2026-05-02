package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proposal #001: Grep モードに「対象ファイル絞り込み (include)」入力欄を追加する。
// 当初 D-2 で採用した rg --glob 路線は ignore 上書き挙動による UX 不整合
// (e.g. `**` で `.git/` まで掘ってしまう) で破綻したため、D-2(b) fuzzy へ転換。
// このファイルは UI 状態（focus 管理 / 入力欄数 / placeholder / Reset 挙動）と
// fuzzy filter 配線のテストをまとめる。

// HeaderViews は pattern と include の 2 入力欄を返す。
// 1 行目は "[Grep]" ラベル + pattern、2 行目は "files:" ラベル + include。
// 親 Model 側はこの 2 行を縦に並べて描画する。
func TestGrepHeaderViewsReturnsPatternAndIncludeLines(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("foo")

	views := g.HeaderViews()
	require.Len(t, views, 2, "Grep は pattern + include の 2 入力欄")
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

	pane, _ = got.Update(keyMsg("C"))
	pane, _ = pane.(*GrepModel).Update(keyMsg("L"))
	pane, _ = pane.(*GrepModel).Update(keyMsg("A"))
	got = pane.(*GrepModel)

	assert.Equal(t, "CLA", got.IncludeValue())
	assert.Equal(t, "hello", got.Query(), "pattern 側は影響を受けない")
}

// include への入力も rg を再発火させる (debounce 経由)。
func TestGrepIncludeInputTriggersDebounce(t *testing.T) {
	g := NewGrepModel()
	g.debounceTag = 0

	// include に focus
	g.Update(specialKeyMsg(tea.KeyShiftTab))

	prevTag := g.debounceTag
	g.Update(keyMsg("C"))

	assert.Greater(t, g.debounceTag, prevTag,
		"include への入力でも debounceTag が進む (検索を再発火)")
	assert.Equal(t, "C", g.IncludeValue(), "include への入力は値に反映される")
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
func TestGrepResetClearsBothInputsAndRestoresPatternFocus(t *testing.T) {
	g := NewGrepModel()
	g.textInput.SetValue("foo")
	g.includeInput.SetValue("CLAUDE")
	// include に focus を移しておく
	g.Update(specialKeyMsg(tea.KeyShiftTab))

	g.Reset()

	assert.Equal(t, "", g.Query(), "pattern クリア")
	assert.Equal(t, "", g.IncludeValue(), "include クリア")
	assert.True(t, g.textInput.Focused(), "Reset 後は pattern にフォーカスが戻る")
	assert.False(t, g.includeInput.Focused())
}

// include 入力欄の placeholder は何が入るか (ファイルパスの絞り込み) を示す。
// fuzzy 化に伴い旧 "*.go !testdata/**" 例文から汎用的な「filter files」表現に変更。
func TestGrepIncludeInputHasPlaceholder(t *testing.T) {
	g := NewGrepModel()
	require.NotEmpty(t, g.includeInput.Placeholder,
		"include 入力欄には placeholder が必要 (discoverability)")
	assert.True(t,
		strings.Contains(strings.ToLower(g.includeInput.Placeholder), "file") ||
			strings.Contains(strings.ToLower(g.includeInput.Placeholder), "path"),
		"placeholder にファイル/パス絞り込みであることが分かる文言: %q", g.includeInput.Placeholder)
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

// proposal #001 fuzzy 路線: GrepModel は SetAllFiles で Finder と同じ
// ファイル列挙結果を共有する。include 入力欄はこの list に対する fuzzy filter として動く。
func TestGrepStoresAllFilesViaSetter(t *testing.T) {
	g := NewGrepModel()
	files := []string{"main.go", "internal/ui/model.go", "CLAUDE.md"}

	g.SetAllFiles(files)

	assert.Equal(t, files, g.allFiles,
		"SetAllFiles で渡された list を保持し、include 入力時の fuzzy filter ソースとして使う")
}

// fuzzy filter のヘルパは include クエリに合致するファイルだけを返す。
// 空クエリは nil (フィルタ無し = 全件検索を意味する) で返す。
// マッチ 0 件も nil。クエリ非空で list が空でも nil。
func TestFuzzyFilterFiles(t *testing.T) {
	files := []string{
		"CLAUDE.md",
		"README.md",
		"internal/ui/model.go",
		"internal/ui/grep_model.go",
	}

	t.Run("empty query returns nil (no filter)", func(t *testing.T) {
		got := fuzzyFilterFiles("", files)
		assert.Nil(t, got, "空クエリは nil = filter 無効と表現する (caller 側で全検索分岐)")
	})

	t.Run("matches by substring", func(t *testing.T) {
		got := fuzzyFilterFiles("CLAUDE", files)
		assert.Equal(t, []string{"CLAUDE.md"}, got)
	})

	t.Run("subsequence match (fuzzy semantics)", func(t *testing.T) {
		got := fuzzyFilterFiles("intui", files)
		assert.Contains(t, got, "internal/ui/model.go",
			"fuzzy なので intui → internal/ui の subsequence で当たる")
	})

	t.Run("no match returns nil", func(t *testing.T) {
		got := fuzzyFilterFiles("ZZZNONEXISTENT", files)
		assert.Nil(t, got)
	})

	t.Run("empty file list returns nil", func(t *testing.T) {
		got := fuzzyFilterFiles("anything", nil)
		assert.Nil(t, got)
	})
}

// debounceTick で include が非空なら fuzzy filter したファイル群が runGrepCmd に渡される
// (handleDebounceTick の rg 呼び出し経路を pin する)。
// Cmd の中身まで覗くのは過剰なので、include の値変化が「fuzzyFilterFiles の戻り値」と一致することと、
// fuzzy 0 マッチ時は loading=false / items=nil で rg を呼ばないことを別 assertion で確認する。
func TestGrepDebounceTickFuzzyFiltersIncludeFiles(t *testing.T) {
	allFiles := []string{
		"CLAUDE.md",
		"README.md",
		"internal/ui/model.go",
	}

	t.Run("include 非空 + fuzzy ヒットありは rg 起動 (loading=true)", func(t *testing.T) {
		g := NewGrepModel()
		g.SetAllFiles(allFiles)
		g.textInput.SetValue("foo")
		g.includeInput.SetValue("CLAUDE")
		g.debounceTag = 1

		_, cmd := g.Update(debounceTickMsg{tag: 1})
		assert.NotNil(t, cmd, "fuzzy ヒットあり → rg 起動 Cmd が返る")
		assert.True(t, g.loading, "rg 起動中なので loading=true")
	})

	t.Run("include 非空 + fuzzy 0 ヒットは rg を呼ばず空結果", func(t *testing.T) {
		g := NewGrepModel()
		g.SetAllFiles(allFiles)
		g.textInput.SetValue("foo")
		g.includeInput.SetValue("ZZZNOMATCH")
		g.items = []string{"stale"} // 残骸が消えるかも検証
		g.debounceTag = 1

		_, cmd := g.Update(debounceTickMsg{tag: 1})
		assert.Nil(t, cmd, "fuzzy ヒット 0 → rg を呼ばない")
		assert.False(t, g.loading, "rg 呼んでないので loading=false")
		assert.Nil(t, g.items, "前回の items は陳腐化扱いでクリア")
	})

	t.Run("include 空時は filter 無し = 全件検索", func(t *testing.T) {
		g := NewGrepModel()
		g.SetAllFiles(allFiles)
		g.textInput.SetValue("foo")
		// includeInput は空のまま
		g.debounceTag = 1

		_, cmd := g.Update(debounceTickMsg{tag: 1})
		assert.NotNil(t, cmd, "include 空 → 通常の全件 rg 検索 Cmd")
		assert.True(t, g.loading)
	})
}
