package ui

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

// #006: HeaderRenderer.HeaderViews() は入力欄の View を 1 要素以上返す。
// 既存の Finder / Grep は単一入力欄なので 1 要素返却。Filtered Grep (proposal #001)
// が 2 要素返すように拡張する前提の slice 化。
func TestHeaderViewsReturnsInputView(t *testing.T) {
	t.Run("finder", func(t *testing.T) {
		f := NewFinderModel()
		f.textInput.SetValue("hello")
		views := f.HeaderViews()
		assert.Len(t, views, 1, "Finder は単一入力欄なので 1 要素")
		assert.Equal(t, f.textInput.View(), views[0])
	})
	t.Run("grep", func(t *testing.T) {
		g := NewGrepModel()
		g.textInput.SetValue("hello")
		views := g.HeaderViews()
		assert.Len(t, views, 1, "現状 Grep は単一入力欄。proposal #001 Phase 2 で 2 要素化")
		assert.Equal(t, g.textInput.View(), views[0])
	})
}
