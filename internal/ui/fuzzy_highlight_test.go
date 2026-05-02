package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proposal #002 Phase 1: highlightAtIndexes は MatchedIndexes 位置の各 rune に
// fuzzyMatchHlStart/End ANSI を挿入する。隣接 index は同一 ANSI ペアで wrap して
// シーケンス数を最小化する (D-1 補足)。
func TestHighlightAtIndexes(t *testing.T) {
	hlS := fuzzyMatchHlStart
	hlE := fuzzyMatchHlEnd

	tests := []struct {
		name    string
		s       string
		indexes []int
		want    string
	}{
		{
			name:    "nil indexes returns unchanged",
			s:       "hello",
			indexes: nil,
			want:    "hello",
		},
		{
			name:    "empty indexes returns unchanged",
			s:       "hello",
			indexes: []int{},
			want:    "hello",
		},
		{
			name:    "single index in middle",
			s:       "abc",
			indexes: []int{1},
			want:    "a" + hlS + "b" + hlE + "c",
		},
		{
			name:    "adjacent indexes merged into single ANSI pair",
			s:       "abcd",
			indexes: []int{1, 2},
			want:    "a" + hlS + "bc" + hlE + "d",
		},
		{
			name:    "index at start",
			s:       "abc",
			indexes: []int{0},
			want:    hlS + "a" + hlE + "bc",
		},
		{
			name:    "index at end",
			s:       "abc",
			indexes: []int{2},
			want:    "ab" + hlS + "c" + hlE,
		},
		{
			name:    "all indexes",
			s:       "abc",
			indexes: []int{0, 1, 2},
			want:    hlS + "abc" + hlE,
		},
		{
			name:    "multi-rune (japanese) uses rune positions not byte positions",
			s:       "あいう",
			indexes: []int{1},
			want:    "あ" + hlS + "い" + hlE + "う",
		},
		{
			name:    "out-of-range index ignored gracefully",
			s:       "abc",
			indexes: []int{0, 99},
			want:    hlS + "a" + hlE + "bc",
		},
		{
			name:    "indexes provided out of order still work (defensive)",
			s:       "abc",
			indexes: []int{2, 0},
			want:    hlS + "a" + hlE + "b" + hlS + "c" + hlE,
		},
		{
			name:    "duplicate indexes deduped",
			s:       "abc",
			indexes: []int{1, 1, 1},
			want:    "a" + hlS + "b" + hlE + "c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightAtIndexes(tt.s, tt.indexes)
			assert.Equal(t, tt.want, got)
		})
	}
}

// fuzzyMatchHlStart / End は当面 grep preview の matchHlStart / End と同じ値。
// 別定数として独立させているのは「将来の付け替え容易化」目的 (proposal #002 D-1)。
// この parity を pin しておき、差別化したくなったら値を変えると test が fail する形で
// 「意図的な分岐」を可視化する。差別化時はこのテスト自体を撤回 or 反転する。
func TestFuzzyMatchHighlightConstantsCurrentlyMatchGrepPreview(t *testing.T) {
	assert.Equal(t, matchHlStart, fuzzyMatchHlStart,
		"D-1 暫定: 値同じ。差別化するときは値変更 + このアサーション撤回")
	assert.Equal(t, matchHlEnd, fuzzyMatchHlEnd)
}

// proposal #002 Phase 2: クエリ非空時の Finder View はマッチした各文字を
// fuzzyMatchHlStart/End で wrap して描画する。
func TestFinderViewHighlightsMatchedRunesWhenQueryNonEmpty(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"main.go"}
	f.loading = false
	f.SetViewSize(5, 80)
	// クエリ "mai" → main.go の m, a, i が連続マッチ
	f.textInput.SetValue("mai")
	f.applyFilter()

	view := f.View()

	// 連続マッチは隣接マージで 1 ペアにまとまる
	assert.Contains(t, view, fuzzyMatchHlStart+"mai"+fuzzyMatchHlEnd,
		"クエリ非空時、連続マッチした 'mai' が単一の ANSI ペアでハイライト")
}

// 空クエリ時はハイライトを完全に skip する (proposal #002 D-3)。
// FuzzyFilter("") は内部仕様で全文字インデックスを返すが、render 側で query を見て
// 「query 非空のとき以外はハイライト skip」する条件で対応する。
func TestFinderViewSkipsHighlightWhenQueryEmpty(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"main.go"}
	f.loading = false
	f.SetViewSize(5, 80)
	// クエリ空 → 全件表示、ハイライトなし
	f.applyFilter()

	view := f.View()

	assert.NotContains(t, view, fuzzyMatchHlStart,
		"空クエリ時はハイライト ANSI を一切出さない")
}

// applyFilter は MatchedIndexes を items に保持する (proposal #002 D-6)。
// View だけでなくテストからも触れるため、データ層の契約として pin する。
func TestFinderApplyFilterStoresMatchedIndexes(t *testing.T) {
	f := NewFinderModel()
	f.allFiles = []string{"main.go", "README.md"}
	f.textInput.SetValue("ma")
	f.applyFilter()

	require.NotEmpty(t, f.items)
	for _, it := range f.items {
		assert.NotEmpty(t, it.MatchedIndexes,
			"クエリ非空でマッチした各 item は MatchedIndexes を持つ: %q", it.Str)
	}
}

// Selector ロール契約 (SelectedItem / FilePath / OpenTarget) は items 型変更後も string を返す。
// 外部から見た振る舞いが変わらないことを pin。
func TestFinderSelectorContractUnchangedAfterFuzzyItemRefactor(t *testing.T) {
	f := NewFinderModel()
	f.items = []fuzzyItem{
		{Str: "main.go"},
		{Str: "go.mod"},
	}
	f.cursor = 1

	assert.Equal(t, "go.mod", f.SelectedItem(), "SelectedItem は文字列を返す")
	assert.Equal(t, "go.mod", f.FilePath(), "FilePath は文字列を返す")
	gotPath, gotLine := f.OpenTarget()
	assert.Equal(t, "go.mod", gotPath)
	assert.Equal(t, 0, gotLine)
}

// proposal #002 Phase 3: Grep モードで include 非空時、左ペインの各 item の
// **ファイルパス部分のみ** をハイライトする。`:line:text` 部分は元々表示されていないため
// テストでは「パス部分にハイライトがあり、それ以外には無い」ことを確認する。
//
// テストは View 単体の振る舞いを pin する目的なので handleDebounceTick (rg 起動経路) を
// 経由せず、pathMatchedIndexes を直接 inject する。debounceTick 経由のフロー検証は
// TestGrepDebounceTickFuzzyFiltersIncludeFiles で別途カバー。
func TestGrepViewHighlightsIncludeMatchInPathPortion(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"CLAUDE.md:10:hello world"}
	g.pathMatchedIndexes = map[string][]int{
		"CLAUDE.md": {0, 1, 2, 3, 4, 5}, // "CLAUDE" 6 文字
	}
	g.cursor = 0
	g.SetViewSize(5, 80)

	view := g.View()

	// "CLAUDE" 6 文字が連続マッチ → 単一 ANSI ペアでハイライト
	assert.Contains(t, view, fuzzyMatchHlStart+"CLAUDE"+fuzzyMatchHlEnd,
		"include 'CLAUDE' のマッチ位置がパス部分に挿入される")
	// テキスト部分はそもそも View に出ない (parseGrepItem で path だけ抽出される)。
	// それでも念のため hello のような text 由来文字列にハイライトが乗っていないことを確認。
	assert.NotContains(t, view, fuzzyMatchHlStart+"hello",
		"テキスト部分はハイライトの対象外")
}

// include が空のときは Grep 左ペインのハイライトを skip。
// pathMatchedIndexes が空 (Reset 直後など) なら何も挿入しない。
func TestGrepViewSkipsHighlightWhenIncludeEmpty(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"CLAUDE.md:10:hello world"}
	g.cursor = 0
	g.SetViewSize(5, 80)
	// includeInput は空、pathMatchedIndexes も空

	view := g.View()
	assert.NotContains(t, view, fuzzyMatchHlStart,
		"include 空時は Grep 左ペインにハイライト ANSI を一切挿入しない")
}

// fuzzyFilterFiles は filter 後のファイル list と、各パス → MatchedIndexes の
// マップの両方を返すよう拡張する (proposal #002 Phase 3)。
// path → indexes マップは Grep 左ペインのファイルパス部分ハイライトに使う。
func TestFuzzyFilterFilesReturnsPathsAndIndexes(t *testing.T) {
	files := []string{"CLAUDE.md", "README.md"}

	t.Run("query 空は (nil, nil)", func(t *testing.T) {
		paths, idx := fuzzyFilterFiles("", files)
		assert.Nil(t, paths)
		assert.Nil(t, idx)
	})

	t.Run("マッチありはパスとインデックスマップを返す", func(t *testing.T) {
		paths, idx := fuzzyFilterFiles("CLAUDE", files)
		assert.Equal(t, []string{"CLAUDE.md"}, paths)
		require.NotNil(t, idx)
		assert.Contains(t, idx, "CLAUDE.md")
		assert.NotEmpty(t, idx["CLAUDE.md"], "CLAUDE.md には MatchedIndexes が紐付く")
	})

	t.Run("マッチ 0 件は (nil, nil)", func(t *testing.T) {
		paths, idx := fuzzyFilterFiles("ZZZNOMATCH", files)
		assert.Nil(t, paths)
		assert.Nil(t, idx)
	})
}
