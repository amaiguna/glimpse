package finder

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzyFilter(t *testing.T) {
	items := []string{
		"internal/ui/model.go",
		"internal/grep/grep.go",
		"internal/finder/fuzzy.go",
		"main.go",
		"README.md",
	}

	tests := []struct {
		name      string
		query     string
		items     []string
		wantStrs  []string // 期待する結果の文字列
		wantEmpty bool     // 結果が空であることを期待
		unordered bool     // true の場合、順序を無視して比較
	}{
		{
			name:     "exact filename match",
			query:    "main.go",
			items:    items,
			wantStrs: []string{"main.go"},
		},
		{
			name:  "partial match returns multiple results",
			query: "go",
			items: items,
			// .go を含むファイルが全てマッチする（スコア順は問わない）
			wantStrs: []string{
				"internal/finder/fuzzy.go",
				"internal/grep/grep.go",
				"internal/ui/model.go",
				"main.go",
			},
			unordered: true,
		},
		{
			name:  "fuzzy match with non-contiguous characters",
			query: "mg",
			items: items,
			// "m" と "g" が非連続でマッチするファイル
			wantStrs:  []string{"main.go", "internal/ui/model.go"},
			unordered: true,
		},
		{
			name:      "no match",
			query:     "zzzzz",
			items:     items,
			wantEmpty: true,
		},
		{
			name:     "empty query returns all items in original order",
			query:    "",
			items:    items,
			wantStrs: items,
		},
		{
			name:     "empty items",
			query:    "foo",
			items:    []string{},
			wantEmpty: true,
		},
		{
			name:     "nil items",
			query:    "foo",
			items:    nil,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FuzzyFilter(tt.query, tt.items)

			if tt.wantEmpty {
				assert.Empty(t, got)
				return
			}

			if tt.wantStrs != nil {
				require.Len(t, got, len(tt.wantStrs))
				if tt.unordered {
					gotStrs := make([]string, len(got))
					for i, m := range got {
						gotStrs[i] = m.Str
					}
					sort.Strings(gotStrs)
					wantSorted := make([]string, len(tt.wantStrs))
					copy(wantSorted, tt.wantStrs)
					sort.Strings(wantSorted)
					assert.Equal(t, wantSorted, gotStrs)
				} else {
					for i, want := range tt.wantStrs {
						assert.Equal(t, want, got[i].Str, "index %d", i)
					}
				}
			} else {
				// wantStrs 未指定の場合は最低1件マッチすることだけ確認
				assert.NotEmpty(t, got)
			}

			// 全結果で MatchedIndexes が設定されていることを確認
			for i, m := range got {
				assert.NotEmpty(t, m.MatchedIndexes, "index %d: MatchedIndexes should not be empty", i)
			}
		})
	}
}
