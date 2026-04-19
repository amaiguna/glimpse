package ui

import (
	"errors"
	"testing"

	"github.com/amaiguna/telescope-tui/internal/grep"
	tea "github.com/charmbracelet/bubbletea"
)

// FuzzModelUpdateView はランダムな Msg 列を Model に送り、
// Update → View の流れで panic が起きないことを検証する。
//
// バイト列を「メッセージ種別 + パラメータ」として解釈し、
// キー入力・リサイズ・ファイルロード・Grep 結果・マウスイベントなどを
// ランダムに混ぜて投げる。
func FuzzModelUpdateView(f *testing.F) {
	// シードコーパス: 典型的な操作パターン
	// kind%8 で種別を決めるので、kind=0:文字, 1:特殊キー, 2:リサイズ, 3:ファイルロード, etc.
	f.Add([]byte{0, 'h', 0, 'e', 0, 'l', 0, 'l', 0, 'o'})         // 文字入力
	f.Add([]byte{1, 4, 1, 4})                                       // Tab 往復（keys[4%18]=KeyEnter → keys[4]=KeyTab にマッピング）
	f.Add([]byte{2, 80, 24, 3, 0, 1, 1, 1, 0})                     // リサイズ→ロード→↓→↓→Enter
	f.Add([]byte{1, 1, 1, 1, 1, 4})                                 // ↓↓↓→Tab
	f.Add([]byte{7, 1, 1, 0, 'x', 6, 50, 20, 0, 'y'})             // 極小リサイズ→文字→マウス→文字
	f.Add([]byte{3, 5, 1, 1, 1, 1, 1, 1, 1, 1, 1, 4, 4, 0, 0, 0}) // ロード→大量↓→Tab→モード切替連打

	f.Fuzz(func(t *testing.T, data []byte) {
		m := NewModel()
		var model tea.Model = m

		i := 0
		for i < len(data) {
			msg := nextMsg(data, &i)
			if msg == nil {
				continue
			}
			result, _ := model.Update(msg)
			model = result

			// View() が panic しないことを検証
			_ = model.(Model).View()
		}
	})
}

// sampleFiles は fuzz テスト内でファイルロードに使うダミーファイル名。
var sampleFiles = []string{
	"main.go", "go.mod", "README.md",
	"internal/ui/model.go", "internal/ui/finder.go",
	"internal/grep/grep.go", "a.go", "b.go", "c.go",
}

// sampleGrepMatches は fuzz テスト内で Grep 結果に使うダミーマッチ。
var sampleGrepMatches = []grep.Match{
	{File: "main.go", Line: 5, Text: "func main() {"},
	{File: "main.go", Line: 10, Text: "package main"},
	{File: "util.go", Line: 20, Text: "func helper() {"},
	{File: "model.go", Line: 100, Text: "func (m Model) Update(msg tea.Msg)"},
}

// nextMsg はバイト列から次の tea.Msg を生成する。
// 先頭バイトでメッセージ種別を選び、後続バイトでパラメータを決める。
func nextMsg(data []byte, pos *int) tea.Msg {
	if *pos >= len(data) {
		return nil
	}
	kind := data[*pos]
	*pos++

	switch kind % 8 {
	case 0: // 文字キー入力
		ch := byte('a')
		if *pos < len(data) {
			ch = data[*pos]
			*pos++
		}
		// 印字可能 ASCII に制限
		if ch < 32 || ch > 126 {
			ch = 'a'
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(ch)}}

	case 1: // 特殊キー
		keyByte := byte(0)
		if *pos < len(data) {
			keyByte = data[*pos]
			*pos++
		}
		keys := []tea.KeyType{
			tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight,
			tea.KeyEnter, tea.KeyTab, tea.KeyEscape,
			tea.KeyBackspace, tea.KeyDelete,
			tea.KeyCtrlA, tea.KeyCtrlC, tea.KeyCtrlE,
			tea.KeyCtrlN, tea.KeyCtrlP,
			tea.KeyHome, tea.KeyEnd,
			tea.KeyPgUp, tea.KeyPgDown,
		}
		return tea.KeyMsg{Type: keys[int(keyByte)%len(keys)]}

	case 2: // ウィンドウリサイズ
		w, h := 80, 24
		if *pos+1 < len(data) {
			w = int(data[*pos])%200 + 10
			h = int(data[*pos+1])%60 + 5
			*pos += 2
		}
		return tea.WindowSizeMsg{Width: w, Height: h}

	case 3: // ファイルロード
		n := len(sampleFiles)
		if *pos < len(data) {
			n = int(data[*pos])%len(sampleFiles) + 1
			*pos++
		}
		return FilesLoadedMsg{Items: sampleFiles[:n]}

	case 4: // Grep 結果
		n := len(sampleGrepMatches)
		if *pos < len(data) {
			n = int(data[*pos])%len(sampleGrepMatches) + 1
			*pos++
		}
		return GrepDoneMsg{Matches: sampleGrepMatches[:n]}

	case 5: // エラー系 Msg
		if *pos < len(data) && data[*pos]%2 == 0 {
			*pos++
			return FilesErrorMsg{Err: errFuzz}
		}
		*pos++
		return GrepErrorMsg{Err: errFuzz}

	case 6: // マウスイベント
		x, y := 10, 5
		if *pos+1 < len(data) {
			x = int(data[*pos]) % 200
			y = int(data[*pos+1]) % 60
			*pos += 2
		}
		return tea.MouseMsg{X: x, Y: y, Type: tea.MouseLeft}

	case 7: // 極小リサイズ（エッジケース）
		w, h := 1, 1
		if *pos+1 < len(data) {
			w = int(data[*pos])%5 + 1
			h = int(data[*pos+1])%5 + 1
			*pos += 2
		}
		return tea.WindowSizeMsg{Width: w, Height: h}
	}

	return nil
}

// errFuzz は fuzz テスト用のダミーエラー。
var errFuzz = errors.New("fuzz error")
