package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/amaiguna/telescope-tui/internal/grep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyMsg はテスト用にキーメッセージを生成するヘルパー。
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// specialKeyMsg は特殊キーのメッセージを生成するヘルパー。
func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestCursorMovement(t *testing.T) {
	tests := []struct {
		name       string
		items      []string
		keys       []tea.KeyMsg
		wantCursor int
	}{
		{
			name:       "↓でカーソルが1つ下に移動する",
			items:      []string{"a", "b", "c"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown)},
			wantCursor: 1,
		},
		{
			name:       "↓を2回でカーソルが2に移動する",
			items:      []string{"a", "b", "c"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown)},
			wantCursor: 2,
		},
		{
			name:       "最下部で↓を押してもカーソルが止まる",
			items:      []string{"a", "b"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown)},
			wantCursor: 1,
		},
		{
			name:       "↑でカーソルが1つ上に移動する",
			items:      []string{"a", "b", "c"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyUp)},
			wantCursor: 1,
		},
		{
			name:       "先頭で↑を押してもカーソルが止まる",
			items:      []string{"a", "b"},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyUp)},
			wantCursor: 0,
		},
		{
			name:       "アイテムが空のとき↓を押してもカーソルが0のまま",
			items:      []string{},
			keys:       []tea.KeyMsg{specialKeyMsg(tea.KeyDown)},
			wantCursor: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.items = tt.items

			var model tea.Model = m
			for _, key := range tt.keys {
				model, _ = model.Update(key)
			}

			got := model.(Model)
			assert.Equal(t, tt.wantCursor, got.cursor)
		})
	}
}

func TestCharacterInput(t *testing.T) {
	tests := []struct {
		name      string
		mode      Mode
		keys      []tea.KeyMsg
		wantQuery string
	}{
		{
			name:      "文字入力でクエリが更新される",
			mode:      ModeFinder,
			keys:      []tea.KeyMsg{keyMsg("a"), keyMsg("b"), keyMsg("c")},
			wantQuery: "abc",
		},
		{
			name:      "BackspaceでQueryの末尾が削除される",
			mode:      ModeFinder,
			keys:      []tea.KeyMsg{keyMsg("a"), keyMsg("b"), specialKeyMsg(tea.KeyBackspace)},
			wantQuery: "a",
		},
		{
			name:      "空のQueryでBackspaceしても空のまま",
			mode:      ModeFinder,
			keys:      []tea.KeyMsg{specialKeyMsg(tea.KeyBackspace)},
			wantQuery: "",
		},
		{
			name:      "Grepモードでも文字入力でクエリが更新される",
			mode:      ModeGrep,
			keys:      []tea.KeyMsg{keyMsg("f"), keyMsg("o"), keyMsg("o")},
			wantQuery: "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.mode = tt.mode

			var model tea.Model = m
			for _, key := range tt.keys {
				model, _ = model.Update(key)
			}

			got := model.(Model)
			assert.Equal(t, tt.wantQuery, got.query)
		})
	}
}

func TestFinderFuzzyFilter(t *testing.T) {
	tests := []struct {
		name      string
		allFiles  []string
		keys      []tea.KeyMsg
		wantItems []string
	}{
		{
			name:      "空クエリで全ファイルが表示される",
			allFiles:  []string{"main.go", "go.mod", "README.md"},
			keys:      []tea.KeyMsg{},
			wantItems: []string{"main.go", "go.mod", "README.md"},
		},
		{
			name:      "クエリで絞り込まれる",
			allFiles:  []string{"main.go", "go.mod", "README.md"},
			keys:      []tea.KeyMsg{keyMsg("m"), keyMsg("a"), keyMsg("i")},
			wantItems: []string{"main.go"},
		},
		{
			name:      "マッチなしで空リストになる",
			allFiles:  []string{"main.go", "go.mod"},
			keys:      []tea.KeyMsg{keyMsg("x"), keyMsg("y"), keyMsg("z")},
			wantItems: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.allFiles = tt.allFiles
			m.items = tt.allFiles

			var model tea.Model = m
			for _, key := range tt.keys {
				model, _ = model.Update(key)
			}

			got := model.(Model)
			assert.Equal(t, tt.wantItems, got.items)
		})
	}
}

func TestModeSwitchWithTab(t *testing.T) {
	tests := []struct {
		name     string
		initial  Mode
		wantMode Mode
	}{
		{
			name:     "FinderからGrepに切り替わる",
			initial:  ModeFinder,
			wantMode: ModeGrep,
		},
		{
			name:     "GrepからFinderに切り替わる",
			initial:  ModeGrep,
			wantMode: ModeFinder,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.mode = tt.initial
			m.query = "something"
			m.cursor = 3
			m.items = []string{"a", "b", "c", "d"}

			model, _ := m.Update(specialKeyMsg(tea.KeyTab))
			got := model.(Model)

			assert.Equal(t, tt.wantMode, got.mode)
			assert.Equal(t, "", got.query, "モード切替でクエリがリセットされる")
			assert.Equal(t, 0, got.cursor, "モード切替でカーソルがリセットされる")
		})
	}
}

func TestCursorClampsOnFilterChange(t *testing.T) {
	m := NewModel()
	m.allFiles = []string{"main.go", "go.mod", "README.md"}
	m.items = m.allFiles
	m.cursor = 2 // README.md を指している

	// "mai" と入力 → main.go のみにフィルタ → カーソルが0にクランプされる
	var model tea.Model = m
	for _, key := range []tea.KeyMsg{keyMsg("m"), keyMsg("a"), keyMsg("i")} {
		model, _ = model.Update(key)
	}

	got := model.(Model)
	assert.Equal(t, 0, got.cursor, "フィルタでアイテムが減ったらカーソルがクランプされる")
}

func TestQuitKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "Ctrl+C で終了", key: tea.KeyMsg{Type: tea.KeyCtrlC}},
		{name: "Esc で終了", key: specialKeyMsg(tea.KeyEscape)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			_, cmd := m.Update(tt.key)
			assert.NotNil(t, cmd, "終了コマンドが返される")
		})
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := model.(Model)
	assert.Equal(t, 120, got.width)
	assert.Equal(t, 40, got.height)
}

// --- Msg ハンドリングテスト ---

func TestFilesLoadedMsg(t *testing.T) {
	m := NewModel()
	m.loading = true

	model, _ := m.Update(FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})
	got := model.(Model)

	assert.Equal(t, []string{"main.go", "go.mod", "README.md"}, got.allFiles)
	assert.Equal(t, []string{"main.go", "go.mod", "README.md"}, got.items)
	assert.False(t, got.loading)
	assert.Nil(t, got.err)
}

func TestFilesLoadedMsgWithExistingQuery(t *testing.T) {
	m := NewModel()
	m.loading = true
	m.query = "mai"

	model, _ := m.Update(FilesLoadedMsg{Items: []string{"main.go", "go.mod", "README.md"}})
	got := model.(Model)

	assert.Equal(t, []string{"main.go", "go.mod", "README.md"}, got.allFiles)
	assert.Equal(t, []string{"main.go"}, got.items, "ロード時にクエリが既にあればフィルタが適用される")
}

func TestFilesErrorMsg(t *testing.T) {
	m := NewModel()
	m.loading = true

	model, _ := m.Update(FilesErrorMsg{Err: errors.New("fd not found")})
	got := model.(Model)

	assert.False(t, got.loading)
	assert.EqualError(t, got.err, "fd not found")
}

func TestGrepDoneMsg(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.loading = true

	matches := []grep.Match{
		{File: "main.go", Line: 10, Text: "func main()"},
		{File: "util.go", Line: 5, Text: "func helper()"},
	}

	model, _ := m.Update(GrepDoneMsg{Matches: matches})
	got := model.(Model)

	assert.Equal(t, []string{"main.go:10:func main()", "util.go:5:func helper()"}, got.items)
	assert.False(t, got.loading)
	assert.Nil(t, got.err)
}

func TestGrepDoneMsgEmpty(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.loading = true

	model, _ := m.Update(GrepDoneMsg{Matches: nil})
	got := model.(Model)

	assert.Nil(t, got.items)
	assert.False(t, got.loading)
}

func TestGrepErrorMsg(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.loading = true

	model, _ := m.Update(GrepErrorMsg{Err: errors.New("rg failed")})
	got := model.(Model)

	assert.False(t, got.loading)
	assert.EqualError(t, got.err, "rg failed")
}

func TestDebounceTickMsg(t *testing.T) {
	t.Run("タグが一致すればCmdが返される", func(t *testing.T) {
		m := NewModel()
		m.mode = ModeGrep
		m.query = "foo"
		m.debounceTag = 5

		_, cmd := m.Update(debounceTickMsg{tag: 5})
		assert.NotNil(t, cmd, "一致するデバウンスタグで Cmd が生成される")
	})

	t.Run("タグが古ければ無視される", func(t *testing.T) {
		m := NewModel()
		m.mode = ModeGrep
		m.query = "foo"
		m.debounceTag = 5

		_, cmd := m.Update(debounceTickMsg{tag: 3})
		assert.Nil(t, cmd, "古いデバウンスタグは無視される")
	})

	t.Run("クエリが空ならCmdなし", func(t *testing.T) {
		m := NewModel()
		m.mode = ModeGrep
		m.query = ""
		m.debounceTag = 5

		model, cmd := m.Update(debounceTickMsg{tag: 5})
		got := model.(Model)
		assert.Nil(t, cmd, "クエリが空なら検索しない")
		assert.Nil(t, got.items, "クエリが空ならアイテムもクリアされる")
	})
}

func TestInitReturnsCmd(t *testing.T) {
	m := NewModel()
	cmd := m.Init()
	assert.NotNil(t, cmd, "Init はファイル列挙コマンドを返す")
}

func TestEnterInFinderMode(t *testing.T) {
	m := NewModel()
	m.mode = ModeFinder
	m.items = []string{"main.go", "go.mod", "README.md"}
	m.cursor = 1

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Enter でエディタ起動コマンドが返される")
}

func TestEnterInGrepMode(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.items = []string{"main.go:10:func main()", "util.go:5:func helper()"}
	m.cursor = 0

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Grep モードの Enter でエディタ起動コマンドが返される")
}

func TestEnterWithNoItems(t *testing.T) {
	m := NewModel()
	m.items = nil

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.Nil(t, cmd, "アイテムがないとき Enter は何もしない")
}

func TestGrepModeQueryChangeReturnsDebounceCmd(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep

	_, cmd := m.Update(keyMsg("f"))
	assert.NotNil(t, cmd, "Grep モードの文字入力でデバウンス Cmd が返される")
}

func TestTabFromGrepToFinderReloadsFiles(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.allFiles = []string{"main.go", "go.mod"}

	model, _ := m.Update(specialKeyMsg(tea.KeyTab))
	got := model.(Model)

	assert.Equal(t, ModeFinder, got.mode)
	assert.Equal(t, []string{"main.go", "go.mod"}, got.items, "Finder に戻ったら allFiles を表示")
}

// --- View テスト ---

func TestViewContainsQuery(t *testing.T) {
	m := NewModel()
	m.query = "main"
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "main", "View にクエリが含まれる")
}

func TestViewContainsItems(t *testing.T) {
	m := NewModel()
	m.items = []string{"main.go", "go.mod", "README.md"}
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "go.mod")
	assert.Contains(t, view, "README.md")
}

func TestViewShowsCursorIndicator(t *testing.T) {
	m := NewModel()
	m.items = []string{"main.go", "go.mod"}
	m.cursor = 1
	m.width = 80
	m.height = 24

	view := m.View()
	// カーソル行にインジケータ ">" が付く
	assert.Contains(t, view, "> go.mod")
}

func TestViewShowsModeLabel(t *testing.T) {
	tests := []struct {
		name  string
		mode  Mode
		label string
	}{
		{name: "Finder モードのラベル", mode: ModeFinder, label: "Files"},
		{name: "Grep モードのラベル", mode: ModeGrep, label: "Grep"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.mode = tt.mode
			m.width = 80
			m.height = 24

			view := m.View()
			assert.Contains(t, view, tt.label)
		})
	}
}

func TestViewShowsError(t *testing.T) {
	m := NewModel()
	m.err = errors.New("something went wrong")
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "something went wrong")
}

func TestViewShowsLoading(t *testing.T) {
	m := NewModel()
	m.loading = true
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "loading", "ロード中表示がある")
}

// --- parseGrepItem テスト ---

func TestParseGrepItem(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFile string
		wantLine int
	}{
		{
			name:     "正常なgrep形式",
			input:    "main.go:10:func main()",
			wantFile: "main.go",
			wantLine: 10,
		},
		{
			name:     "コロンなしの文字列",
			input:    "main.go",
			wantFile: "main.go",
			wantLine: 0,
		},
		{
			name:     "行番号が非数値",
			input:    "main.go:abc:func main()",
			wantFile: "main.go:abc:func main()",
			wantLine: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, line := parseGrepItem(tt.input)
			assert.Equal(t, tt.wantFile, file)
			assert.Equal(t, tt.wantLine, line)
		})
	}
}

// --- プレビュー統合テスト ---

// stripANSI は ANSI エスケープシーケンスを除去するテスト用ヘルパー。
func stripANSI(s string) string {
	result := strings.Builder{}
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func TestPreviewUpdatesOnCursorMove(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	require.NoError(t, os.WriteFile(fileA, []byte("package a\n"), 0644))
	require.NoError(t, os.WriteFile(fileB, []byte("package b\n"), 0644))

	m := NewModel()
	m.items = []string{fileA, fileB}
	m.cursor = 0
	m.width = 80
	m.height = 24
	m.updatePreview()

	assert.Contains(t, stripANSI(m.previewContent), "package a")

	// カーソルを↓に動かすとプレビューが b に変わる
	model, _ := m.Update(specialKeyMsg(tea.KeyDown))
	got := model.(Model)
	assert.Equal(t, 1, got.cursor)
	assert.Contains(t, stripANSI(got.previewContent), "package b")
}

func TestPreviewClearsWhenNoItems(t *testing.T) {
	m := NewModel()
	m.items = nil
	m.previewContent = "stale preview"
	m.updatePreview()

	assert.Equal(t, "", m.previewContent)
}

func TestPreviewInGrepMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(file, []byte("package main\nfunc main() {}\n"), 0644))

	m := NewModel()
	m.mode = ModeGrep
	m.items = []string{file + ":1:package main"}
	m.cursor = 0
	m.updatePreview()

	assert.Contains(t, stripANSI(m.previewContent), "package main")
}

func TestViewContainsPreview(t *testing.T) {
	m := NewModel()
	m.items = []string{"main.go"}
	m.cursor = 0
	m.width = 80
	m.height = 24
	m.previewContent = "package main\nfunc main() {}\n"

	view := m.View()
	assert.Contains(t, view, "package main")
}

func TestViewPreviewLineTruncatedToFitWidth(t *testing.T) {
	m := NewModel()
	m.items = []string{"a.txt"}
	m.cursor = 0
	m.width = 60
	m.height = 10
	// プレビューに幅を超える長い行を設定
	m.previewContent = strings.Repeat("x", 200) + "\n"

	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		// 各行の視覚的な幅がターミナル幅を超えないことを確認（ANSI 対応）
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, m.width,
			"行がターミナル幅を超えている（visual width=%d）: %q", w, stripANSI(line))
	}
}

func TestViewFillsFullHeight(t *testing.T) {
	m := NewModel()
	m.items = []string{"a.txt"}
	m.cursor = 0
	m.width = 80
	m.height = 20
	m.previewContent = "short\n"

	view := m.View()
	// 末尾の改行を除いて行数をカウント
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	// ヘッダー行(1) + コンテンツ行(height-2) = height-1 以上の行数があること
	assert.GreaterOrEqual(t, len(lines), m.height-1,
		"View() がターミナル高さ分の行を出力していない（古い描画が残る原因）")
}

func TestViewPanesDoNotOverlap(t *testing.T) {
	m := NewModel()
	m.items = []string{"main.go", "go.mod", "README.md"}
	m.cursor = 0
	m.width = 80
	m.height = 24
	// ANSI エスケープ付きの長いプレビュー（chroma 出力を模擬���
	m.previewContent = "\x1b[38;5;197mpackage\x1b[0m \x1b[38;5;148mmain\x1b[0m\n" +
		strings.Repeat("\x1b[38;5;231m"+strings.Repeat("x", 100)+"\x1b[0m\n", 20)

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, m.width,
			"行 %d がターミナル幅を超えている（visual width=%d）", i, w)
	}
}
