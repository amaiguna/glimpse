package finder

import (
	"github.com/sahilm/fuzzy"
)

// FuzzyMatch はファジーフィルタの結果1件を表す。
type FuzzyMatch struct {
	Str            string // マッチした元の文字列
	Index          int    // 元のスライス内でのインデックス
	MatchedIndexes []int  // マッチした文字の位置（ハイライト用）
}

// FuzzyFilter はクエリに基づいてアイテムをファジーフィルタリングし、スコア順に返す。
// クエリが空の場合は全アイテムをそのまま返す。
func FuzzyFilter(query string, items []string) []FuzzyMatch {
	if query == "" {
		result := make([]FuzzyMatch, len(items))
		for i, s := range items {
			indexes := make([]int, len(s))
			for j := range s {
				indexes[j] = j
			}
			result[i] = FuzzyMatch{
				Str:            s,
				Index:          i,
				MatchedIndexes: indexes,
			}
		}
		return result
	}

	matches := fuzzy.Find(query, items)
	if len(matches) == 0 {
		return nil
	}

	result := make([]FuzzyMatch, len(matches))
	for i, m := range matches {
		result[i] = FuzzyMatch{
			Str:            m.Str,
			Index:          m.Index,
			MatchedIndexes: m.MatchedIndexes,
		}
	}
	return result
}
