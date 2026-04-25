package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/amaiguna/glimpse-tui/internal/grep"
	"github.com/amaiguna/glimpse-tui/internal/preview"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyMsg はテスト用にキーメ���セージを生成するヘルパー。
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// specialKeyMsg は特殊キーのメッセージを生成するヘルパー。
func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

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

// --- 親 Model: モード切替テスト ---

func TestModeSwitchWithTab(t *testing.T) {
	m := NewModel()
	m.finderPane.allFiles = []string{"a", "b"}
	m.finderPane.items = []string{"a", "b"}

	// Finder → Grep
	model, _ := m.Update(specialKeyMsg(tea.KeyTab))
	got := model.(Model)
	assert.Equal(t, ModeGrep, got.mode)

	// Grep → Finder (allFiles が復元される)
	model, _ = got.Update(specialKeyMsg(tea.KeyTab))
	got = model.(Model)
	assert.Equal(t, ModeFinder, got.mode)
	assert.Equal(t, []string{"a", "b"}, got.finderPane.items)
}

func TestModeSwitchAfterScrollDoesNotPanic(t *testing.T) {
	m := NewModel()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m3, _ := m2.Update(FilesLoadedMsg{Items: []string{"a", "b", "c", "d", "e"}})

	// カーソルを下に移動
	cur := m3
	for i := 0; i < 4; i++ {
		cur, _ = cur.Update(specialKeyMsg(tea.KeyDown))
	}

	// Tab で Grep に切り替え — panic しないこと
	assert.NotPanics(t, func() {
		model, _ := cur.Update(specialKeyMsg(tea.KeyTab))
		got := model.(Model)
		// View() も panic しないこと
		_ = got.View()
	})
}

// --- 親 Model: グローバルキーテスト ---

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
			assert.NotNil(t, cmd, "終了コマンド���返される")
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

func TestInitReturnsCmd(t *testing.T) {
	m := NewModel()
	cmd := m.Init()
	assert.NotNil(t, cmd, "Init はファイル列挙コマンドを返す")
}

// --- 親 Model: Enter キーテスト ---

func TestEnterInFinderMode(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod"}
	m.finderPane.cursor = 1

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Enter でエディタ起動コマンドが返される")
}

func TestEnterInGrepMode(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.items = []string{"main.go:10:func main()"}
	m.grepPane.cursor = 0

	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.NotNil(t, cmd, "Grep モードの Enter でエディタ起動コマンドが返される")
}

func TestEnterWithNoItems(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(specialKeyMsg(tea.KeyEnter))
	assert.Nil(t, cmd, "アイテムがないとき Enter は何もしない")
}

// --- 親 Model: Msg 委譲テスト ---

func TestFilesLoadedMsgDelegatesToFinder(t *testing.T) {
	m := NewModel()

	model, _ := m.Update(FilesLoadedMsg{Items: []string{"a.go", "b.go"}})
	got := model.(Model)

	assert.Equal(t, []string{"a.go", "b.go"}, got.finderPane.allFiles)
	assert.Equal(t, []string{"a.go", "b.go"}, got.finderPane.items)
	assert.False(t, got.finderPane.loading)
}

func TestGrepDoneMsgDelegatesToGrep(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep

	model, _ := m.Update(GrepDoneMsg{Matches: []grep.Match{
		{File: "a.go", Line: 1, Text: "foo"},
	}})
	got := model.(Model)

	assert.Equal(t, []string{"a.go:1:foo"}, got.grepPane.items)
}

func TestCharacterInputDelegatesToActivePane(t *testing.T) {
	m := NewModel()
	m.finderPane.allFiles = []string{"main.go", "go.mod"}
	m.finderPane.items = m.finderPane.allFiles

	model, _ := m.Update(keyMsg("m"))
	got := model.(Model)

	assert.Equal(t, "m", got.finderPane.Query())
}

func TestCursorMoveDelegatesToActivePane(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"a", "b", "c"}

	model, _ := m.Update(specialKeyMsg(tea.KeyDown))
	got := model.(Model)

	assert.Equal(t, 1, got.finderPane.cursor)
}

// --- 親 Model: プレビュー統合テスト ---

// drainBatchCmds は tea.Batch が返す Cmd を全て実行し、
// PreviewLoadedMsg があれば Model に適用するヘルパー。
func drainCmds(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	// tea.Batch は BatchMsg ([]Cmd) を返す
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainCmds(t, m, c)
		}
		return m
	}
	if _, ok := msg.(PreviewLoadedMsg); ok {
		result, _ := m.Update(msg)
		return result.(Model)
	}
	return m
}

func TestPreviewUpdatesOnCursorMove(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	require.NoError(t, os.WriteFile(fileA, []byte("package a\n"), 0644))
	require.NoError(t, os.WriteFile(fileB, []byte("package b\n"), 0644))

	m := NewModel()
	m.finderPane.items = []string{fileA, fileB}
	m.finderPane.allFiles = []string{fileA, fileB}
	m.finderPane.loading = false

	model, cmd := m.Update(specialKeyMsg(tea.KeyDown))
	got := drainCmds(t, model.(Model), cmd)
	assert.Contains(t, stripANSI(got.previewContent), "package b")
}

func TestPreviewClearsWhenNoItems(t *testing.T) {
	m := NewModel()
	m.previewContent = "stale"
	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)
	assert.Equal(t, "", got.previewContent)
}

func TestPreviewBinaryFileInFinderMode(t *testing.T) {
	dir := t.TempDir()
	binFile := filepath.Join(dir, "app.bin")
	require.NoError(t, os.WriteFile(binFile, []byte{0x7f, 'E', 'L', 'F', 0x00, 'x'}, 0644))

	m := NewModel()
	m.finderPane.items = []string{binFile}
	m.finderPane.allFiles = []string{binFile}
	m.finderPane.loading = false

	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.Equal(t, preview.BinaryFileMessage, got.previewContent)
}

func TestPreviewBinaryFileInGrepMode(t *testing.T) {
	dir := t.TempDir()
	binFile := filepath.Join(dir, "app.bin")
	require.NoError(t, os.WriteFile(binFile, []byte{0x00, 0x01, 0x02}, 0644))

	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.items = []string{binFile + ":1:match"}
	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.Equal(t, preview.BinaryFileMessage, got.previewContent)
}

// M-1 回帰: MaxPreviewSize を超えるテキストファイルは LargeFileMessage を返す。
func TestPreviewLargeFileInFinderMode(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "huge.txt")
	require.NoError(t, os.WriteFile(bigFile, make([]byte, preview.MaxPreviewSize+1), 0644))

	m := NewModel()
	m.finderPane.items = []string{bigFile}
	m.finderPane.allFiles = []string{bigFile}
	m.finderPane.loading = false

	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.Equal(t, preview.LargeFileMessage, got.previewContent)
}

func TestPreviewLargeFileInGrepMode(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "huge.log")
	require.NoError(t, os.WriteFile(bigFile, make([]byte, preview.MaxPreviewSize+1), 0644))

	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.items = []string{bigFile + ":1:match"}
	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.Equal(t, preview.LargeFileMessage, got.previewContent)
}

// 上限ちょうどはまだプレビュー対象（境界条件）。
func TestPreviewExactlyMaxSizeIsAllowed(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "edge.txt")
	content := make([]byte, preview.MaxPreviewSize)
	for i := range content {
		content[i] = 'a'
	}
	require.NoError(t, os.WriteFile(file, content, 0644))

	m := NewModel()
	m.finderPane.items = []string{file}
	m.finderPane.allFiles = []string{file}
	m.finderPane.loading = false

	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.NotEqual(t, preview.LargeFileMessage, got.previewContent)
}

func TestPreviewInGrepMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(file, []byte("package main\n"), 0644))

	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.items = []string{file + ":1:package main"}
	cmd := m.previewCmd()
	got := drainCmds(t, m, cmd)

	assert.Contains(t, stripANSI(got.previewContent), "package main")
}

func TestGrepPreviewRange(t *testing.T) {
	tests := []struct {
		name          string
		item          string
		visibleHeight int
		want          int
	}{
		{"中央配置", "main.go:50:text", 20, 40},
		{"先頭クランプ", "main.go:3:text", 20, 1},
		{"1行目", "main.go:1:text", 20, 1},
		{"アイテムなし", "", 20, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGrepModel()
			if tt.item != "" {
				g.items = []string{tt.item}
			}
			got := g.PreviewRange(tt.visibleHeight)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGrepDecoratePreviewTargetsCorrectLine(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:2:func main()"}
	g.textInput.SetValue("main")

	// PreviewRange を呼んで previewStartLine をセット
	g.PreviewRange(20)

	content := "package main\nfunc main() {\n}\n"
	result := g.DecoratePreview(content, 40)

	lines := strings.Split(result, "\n")
	// 非ヒット行は変化なし
	assert.Equal(t, "package main", stripANSI(lines[0]))
	// ヒット行のテキストは保持されている
	assert.Equal(t, "func main() {", stripANSI(lines[1]))
	assert.Equal(t, "}", stripANSI(lines[2]))
}

func TestGrepDecoratePreviewWithOffset(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:50:target line"}
	g.textInput.SetValue("target")

	// visibleHeight=20 → startLine=40
	startLine := g.PreviewRange(20)
	assert.Equal(t, 40, startLine)

	// プレビューは40行目から表示されている想定
	// 50行目は表示上のインデックス10 (50-40=10)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", startLine+i)
	}
	content := strings.Join(lines, "\n")
	result := g.DecoratePreview(content, 80)

	resultLines := strings.Split(result, "\n")
	// 10番目の行 (0-indexed) がハイライト対象（stripANSI で元テキストが残る）
	assert.Equal(t, "line 50", stripANSI(resultLines[10]))
	// ハイライト対象外の行は変更なし
	assert.Equal(t, "line 49", resultLines[9])
}

func TestGrepDecoratePreviewEmptyContent(t *testing.T) {
	g := NewGrepModel()
	g.items = []string{"main.go:1:package main"}
	g.textInput.SetValue("package")
	g.PreviewRange(20)
	assert.Equal(t, "", g.DecoratePreview("", 40))
}

func TestGrepDecoratePreviewNoItems(t *testing.T) {
	g := NewGrepModel()
	content := "package main\n"
	assert.Equal(t, content, g.DecoratePreview(content, 40))
}

func TestHighlightMatches(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		query    string
		wantText string // stripANSI 後のテキスト（変わらないこと）
		changed  bool   // 元の行から変化があるか
	}{
		{
			name:     "単純マッチ",
			line:     "func main() {",
			query:    "main",
			wantText: "func main() {",
			changed:  true,
		},
		{
			name:     "大文字小文字を区別しない",
			line:     "Package Main",
			query:    "package",
			wantText: "Package Main",
			changed:  true,
		},
		{
			name:     "マッチなし",
			line:     "func main() {",
			query:    "xyz",
			wantText: "func main() {",
			changed:  false,
		},
		{
			name:     "複数マッチ",
			line:     "aa bb aa",
			query:    "aa",
			wantText: "aa bb aa",
			changed:  true,
		},
		{
			name:     "空クエリ",
			line:     "func main() {",
			query:    "",
			wantText: "func main() {",
			changed:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := highlightMatches(tt.line, tt.query)
			assert.Equal(t, tt.wantText, stripANSI(result), "テキスト内容が保持されること")
			if tt.changed {
				assert.NotEqual(t, tt.line, result, "ハイライトが適用されること")
			} else {
				assert.Equal(t, tt.line, result, "変更がないこと")
			}
		})
	}
}

func TestHighlightMatchesPreservesANSI(t *testing.T) {
	// シンタックスハイライト済み行（chroma 風）
	line := "\x1b[38;5;81mpackage\x1b[0m \x1b[38;5;166mmain\x1b[0m"
	result := highlightMatches(line, "main")

	// テキスト内容は保持
	assert.Equal(t, "package main", stripANSI(result))
	// 元の前景色シーケンスが残っている
	assert.Contains(t, result, "\x1b[38;5;166m", "chroma の前景色が保持されること")
	// ハイライト開始シーケンスが含まれている
	assert.Contains(t, result, matchHlStart, "マッチハイライトが適用されること")
}

func TestFinderDecoratePreviewPassthrough(t *testing.T) {
	f := NewFinderModel()
	content := "package main\nfunc main() {\n}\n"
	result := f.DecoratePreview(content, 40)
	assert.Equal(t, content, result, "Finder モードではプレビューを加工しない")
}

// --- View テスト ---

func TestViewContainsQuery(t *testing.T) {
	m := NewModel()
	m.finderPane.textInput.SetValue("main")
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "main")
}

func TestViewContainsItems(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod"}
	m.finderPane.loading = false
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "go.mod")
}

func TestViewShowsCursorIndicator(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod"}
	m.finderPane.cursor = 1
	m.finderPane.loading = false
	m.width = 80
	m.height = 24

	view := m.View()
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
			m.finderPane.loading = false
			m.width = 80
			m.height = 24

			view := m.View()
			assert.Contains(t, view, tt.label)
		})
	}
}

func TestViewShowsError(t *testing.T) {
	m := NewModel()
	m.finderPane.err = errors.New("something went wrong")
	m.finderPane.loading = false
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "something went wrong")
}

func TestViewContainsPreview(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go"}
	m.finderPane.loading = false
	m.previewContent = "package main\nfunc main() {}\n"
	m.width = 80
	m.height = 24

	view := m.View()
	assert.Contains(t, view, "package main")
}

func TestViewPreviewLineTruncatedToFitWidth(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"a.txt"}
	m.finderPane.loading = false
	m.previewContent = strings.Repeat("x", 200) + "\n"
	m.width = 60
	m.height = 10

	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, m.width,
			"行がターミナル幅を超えている（visual width=%d）", w)
	}
}

func TestViewFillsFullHeight(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"a.txt"}
	m.finderPane.loading = false
	m.previewContent = "short\n"
	m.width = 80
	m.height = 20

	view := m.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	assert.GreaterOrEqual(t, len(lines), m.height-1,
		"View() がタ��ミナル高さ分の行を出力していない")
}

func TestViewPanesDoNotOverlap(t *testing.T) {
	m := NewModel()
	m.finderPane.items = []string{"main.go", "go.mod", "README.md"}
	m.finderPane.loading = false
	m.previewContent = "\x1b[38;5;197mpackage\x1b[0m \x1b[38;5;148mmain\x1b[0m\n" +
		strings.Repeat("\x1b[38;5;231m"+strings.Repeat("x", 100)+"\x1b[0m\n", 20)
	m.width = 80
	m.height = 24

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, m.width,
			"行 %d がターミナル幅を超えている（visual width=%d）", i, w)
	}
}

// --- parseGrepItem テスト ---

func TestParseGrepItem(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFile string
		wantLine int
	}{
		{name: "正常なgrep形式", input: "main.go:10:func main()", wantFile: "main.go", wantLine: 10},
		{name: "コロンなしの文字列", input: "main.go", wantFile: "main.go", wantLine: 0},
		{name: "行番号が非数値", input: "main.go:abc:func main()", wantFile: "main.go:abc:func main()", wantLine: 0},
		// I-2: 右側 (line:text) を rsplit で取り出すので、ファイルパスに `:` が含まれていてもよい。
		{name: "Windowsパス", input: `C:\foo\bar.go:10:hello`, wantFile: `C:\foo\bar.go`, wantLine: 10},
		{name: "ファイル名にコロンを含む", input: "weird:name.txt:5:matched", wantFile: "weird:name.txt", wantLine: 5},
		// text 部分にコロンがあっても影響しない（line の直後の最初の `:` で区切られる）。
		{name: "textに複数のコロン", input: "main.go:42:foo:bar:baz", wantFile: "main.go", wantLine: 42},
		// text が空でも file:line: の形なら認識される。
		{name: "text空", input: "main.go:7:", wantFile: "main.go", wantLine: 7},
		// I-2 fuzz 検出: 行番号は 1-based。`:0:` や `:00` は parse 失敗扱い。
		{name: "ファイル空+line0", input: ":00", wantFile: ":00", wantLine: 0},
		{name: "line0は非マッチ", input: "main.go:0:hi", wantFile: "main.go:0:hi", wantLine: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, line := parseGrepItem(tt.input)
			assert.Equal(t, tt.wantFile, file)
			assert.Equal(t, tt.wantLine, line)
		})
	}
}

// FuzzParseGrepItem は parseGrepItem の不変条件を fuzz で検証する。
// 任意の文字列入力に対し:
//   - panic しない
//   - line >= 0（負の行番号は出ない）
//   - file は input の prefix（あるいは input そのもの）
//   - line > 0 のとき: input は file + ":" の後に digit列 + (":" or 文末) という構造を持つ
func FuzzParseGrepItem(f *testing.F) {
	// シード: 既存テストと、エッジケース
	seeds := []string{
		"main.go:10:func main()",
		"main.go",
		"main.go:abc:hi",
		`C:\foo\bar.go:10:hello`,
		"weird:name.txt:5:matched",
		"main.go:42:foo:bar:baz",
		"main.go:7:",
		"",
		":",
		"::",
		":::",
		":1:",
		"a:9999999999999999999999999:x", // overflow
		"\x00:1:\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		file, line := parseGrepItem(input)

		if line < 0 {
			t.Errorf("negative line %d for input %q", line, input)
		}
		if !strings.HasPrefix(input, file) {
			t.Errorf("file %q is not a prefix of input %q", file, input)
		}
		if line == 0 {
			// 解釈失敗ケースでは file == input を満たす実装。
			if file != input {
				t.Errorf("on line==0 expected file==input, got file=%q input=%q", file, input)
			}
			return
		}
		// line > 0 の reconstruction 検証:
		// input[len(file)] == ':' であり、その直後から digit 列が始まり、
		// digit 列を strconv.Atoi すると line と一致し、その直後は ':' か文末。
		rest := input[len(file):]
		if !strings.HasPrefix(rest, ":") {
			t.Errorf("expected ':' after file, got %q (input=%q)", rest, input)
			return
		}
		rest = rest[1:]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end == 0 {
			t.Errorf("expected digit run after ':' (input=%q)", input)
			return
		}
		got, err := strconv.Atoi(rest[:end])
		if err != nil {
			t.Errorf("digit run failed to parse: %q (input=%q)", rest[:end], input)
			return
		}
		if got != line {
			t.Errorf("reconstructed line %d != returned line %d (input=%q)", got, line, input)
		}
		if end < len(rest) && rest[end] != ':' {
			t.Errorf("expected ':' or EOS after digits, got %q (input=%q)", rest[end:], input)
		}
	})
}
