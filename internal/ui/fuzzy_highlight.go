package ui

import "strings"

// highlightAtIndexes は s の各 rune 位置 (indexes に含まれるもの) を
// fuzzyMatchHlStart / fuzzyMatchHlEnd で wrap して返す (proposal #002 D-1)。
//
//   - indexes は **rune** 位置 (sahilm/fuzzy が返す MatchedIndexes と同じ単位)
//   - 隣接 index は同一 ANSI ペアでまとめて wrap し、シーケンス数を最小化する
//   - 範囲外 index / 重複 index / 順序未整列 すべて defensive に許容
//   - indexes が空のときは入力をそのまま返す (ANSI を一切挿入しない)
func highlightAtIndexes(s string, indexes []int) string {
	if len(indexes) == 0 {
		return s
	}
	idxSet := make(map[int]struct{}, len(indexes))
	for _, i := range indexes {
		idxSet[i] = struct{}{}
	}

	var b strings.Builder
	b.Grow(len(s) + len(idxSet)*(len(fuzzyMatchHlStart)+len(fuzzyMatchHlEnd)))

	runeIdx := 0
	inHl := false
	for _, r := range s {
		_, hit := idxSet[runeIdx]
		if hit && !inHl {
			b.WriteString(fuzzyMatchHlStart)
			inHl = true
		} else if !hit && inHl {
			b.WriteString(fuzzyMatchHlEnd)
			inHl = false
		}
		b.WriteRune(r)
		runeIdx++
	}
	if inHl {
		b.WriteString(fuzzyMatchHlEnd)
	}
	return b.String()
}
