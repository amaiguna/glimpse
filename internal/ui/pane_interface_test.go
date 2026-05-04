package ui

import (
	"os"
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

// TestMain は lipgloss の color profile をテスト中に明示的に ANSI256 へ固定する。
// 非 TTY 環境（go test の実行コンテキスト）では lipgloss が自動で NoColor になり、
// modeLabelStyle / inactiveLabelStyle の Render 結果が同じプレーン文字列になってしまう。
// これだと「active/inactive ラベル切替」のような色差を assertion で検証できない。
// goldenTest は stripANSI で ANSI を除去してから比較するため、color profile を強制しても
// 既存ゴールデンには影響しない。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	os.Exit(m.Run())
}

// #006: Pane インターフェース分割の compile-time 契約。
// FinderModel / GrepModel が Pane / HeaderRenderer / Selector / PreviewDecorator
// のすべてを満たすことを型レベルで pin する。将来 Pane を分割し直したり
// メソッドシグネチャを変えたりするとここで即落ちる。
var (
	_ Pane             = (*FinderModel)(nil)
	_ HeaderRenderer   = (*FinderModel)(nil)
	_ Selector         = (*FinderModel)(nil)
	_ PreviewDecorator = (*FinderModel)(nil)

	_ Pane             = (*GrepModel)(nil)
	_ HeaderRenderer   = (*GrepModel)(nil)
	_ Selector         = (*GrepModel)(nil)
	_ PreviewDecorator = (*GrepModel)(nil)
)

// #006: Pane インターフェース本体は 6 メソッドに絞られる。
// SelectedItem / FilePath / OpenTarget / PreviewRange / DecoratePreview /
// HeaderViews(旧 TextInputView) は別ロールに切り出されている。
// メソッドが増えたら ISP 違反として再分割を検討する境界（issue #006 の判断ライン）。
func TestPaneInterfaceMethodCount(t *testing.T) {
	typ := reflect.TypeOf((*Pane)(nil)).Elem()
	want := 6
	assert.Equal(t, want, typ.NumMethod(),
		"Pane interface should have exactly %d methods (core role only); got %d. "+
			"オプショナルなロールは HeaderRenderer / Selector / PreviewDecorator に切り出すこと",
		want, typ.NumMethod())
}

// #006: HeaderRenderer.HeaderViews() は「ラベル + 入力欄 View」を含む完成行を返す。
// Finder は単一入力欄、Grep は proposal #001 Phase 2 で pattern + include の 2 要素返却。
// ラベルの active/inactive スタイル切替は pane の責任 (詳細は別テスト参照)。
func TestHeaderViewsReturnsInputView(t *testing.T) {
	t.Run("finder", func(t *testing.T) {
		f := NewFinderModel()
		f.textInput.SetValue("hello")
		views := f.HeaderViews()
		assert.Len(t, views, 1, "Finder は単一入力欄なので 1 要素")
		assert.Contains(t, views[0], f.textInput.View(),
			"1 要素目に textinput.View() が含まれる")
		assert.Contains(t, views[0], "[Files]", "Finder のモードラベルが含まれる")
	})
	t.Run("grep", func(t *testing.T) {
		g := NewGrepModel()
		g.textInput.SetValue("hello")
		views := g.HeaderViews()
		assert.Len(t, views, 2, "Grep は pattern + include の 2 要素 (proposal #001 Phase 2)")
		assert.Contains(t, views[0], g.textInput.View(), "1 行目に pattern textinput.View()")
		assert.Contains(t, views[0], "[Grep]", "1 行目にモードラベル [Grep]")
		assert.Contains(t, views[1], "[Path]", "2 行目に include ラベル [Path]")
	})
}
