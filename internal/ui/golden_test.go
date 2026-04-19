package ui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "ゴールデンファイルを更新する")

// goldenTest はゴールデンテストのヘルパー。
// View() の出力から ANSI エスケープを除去し、ゴールデンファイルと比較する。
func goldenTest(t *testing.T, name string, view string) {
	t.Helper()
	got := stripANSI(view)

	goldenDir := filepath.Join("testdata", "golden")
	goldenFile := filepath.Join(goldenDir, name+".txt")

	if *update {
		require.NoError(t, os.MkdirAll(goldenDir, 0755))
		require.NoError(t, os.WriteFile(goldenFile, []byte(got), 0644))
		t.Logf("updated golden file: %s", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	require.NoError(t, err, "ゴールデンファイルが見つかりません。-update フラグで生成してください")
	assert.Equal(t, string(want), got)
}

// setupModel はテスト用モデルを構築するヘルパー。
func setupModel(files []string, width, height int, preview string) Model {
	m := NewModel()
	m2, _ := m.Update(FilesLoadedMsg{Items: files})
	m3, _ := m2.Update(tea.WindowSizeMsg{Width: width, Height: height})
	got := m3.(Model)
	got.previewContent = preview
	return got
}

// setupGrepModel はテスト用 Grep モードのモデルを構築するヘルパー。
func setupGrepModel(query string, items []string, width, height int, preview string) Model {
	m := NewModel()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	got := m2.(Model)
	got.mode = ModeGrep
	got.grepPane.textInput.SetValue(query)
	got.grepPane.items = items
	got.previewContent = preview
	return got
}

// --- ゴールデンテスト ---

func TestGoldenFinderEmpty(t *testing.T) {
	m := setupModel(nil, 80, 24, "")
	goldenTest(t, "finder_empty", m.View())
}

func TestGoldenFinderFewItems(t *testing.T) {
	m := setupModel([]string{"main.go", "go.mod"}, 80, 24, "package main\n\nfunc main() {\n}\n")
	goldenTest(t, "finder_few_items", m.View())
}

func TestGoldenFinderManyItems(t *testing.T) {
	var files []string
	for i := 0; i < 50; i++ {
		files = append(files, fmt.Sprintf("pkg/service/handler_%02d.go", i))
	}
	m := setupModel(files, 80, 24, "package service\n\nfunc Handle() error {\n\treturn nil\n}\n")
	goldenTest(t, "finder_many_items", m.View())
}

func TestGoldenFinderManyItemsScrolled(t *testing.T) {
	var files []string
	for i := 0; i < 50; i++ {
		files = append(files, fmt.Sprintf("pkg/service/handler_%02d.go", i))
	}
	m := setupModel(files, 80, 24, "")

	// カーソルを30行目まで移動
	var cur tea.Model = m
	for i := 0; i < 30; i++ {
		cur, _ = cur.Update(specialKeyMsg(tea.KeyDown))
	}
	got := cur.(Model)
	got.previewContent = "package service\n"
	goldenTest(t, "finder_many_items_scrolled", got.View())
}

func TestGoldenPreviewShort(t *testing.T) {
	m := setupModel([]string{"short.txt"}, 80, 24, "hello\n")
	goldenTest(t, "preview_short", m.View())
}

func TestGoldenPreviewLong(t *testing.T) {
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d: some content here for testing purposes", i))
	}
	m := setupModel([]string{"long.txt"}, 80, 24, strings.Join(lines, "\n"))
	goldenTest(t, "preview_long", m.View())
}

func TestGoldenWindowSmall(t *testing.T) {
	files := []string{"main.go", "go.mod", "go.sum", "internal/ui/model.go", "internal/ui/finder.go"}
	m := setupModel(files, 40, 12, "package main\n\nfunc main() {\n}\n")
	goldenTest(t, "window_small", m.View())
}

func TestGoldenWindowLarge(t *testing.T) {
	var files []string
	for i := 0; i < 20; i++ {
		files = append(files, fmt.Sprintf("internal/pkg/module_%02d.go", i))
	}
	var previewLines []string
	for i := 1; i <= 40; i++ {
		previewLines = append(previewLines, fmt.Sprintf("// line %d", i))
	}
	m := setupModel(files, 160, 50, strings.Join(previewLines, "\n"))
	goldenTest(t, "window_large", m.View())
}

func TestGoldenGrepEmpty(t *testing.T) {
	m := NewModel()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	got := m2.(Model)
	// Grep モードに切り替え
	m3, _ := got.Update(specialKeyMsg(tea.KeyTab))
	goldenTest(t, "grep_empty", m3.(Model).View())
}

func TestGoldenGrepWithResults(t *testing.T) {
	m := setupGrepModel(
		"func",
		[]string{
			"main.go:5:func main() {",
			"internal/ui/model.go:73:func NewModel() Model {",
			"internal/ui/model.go:95:func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {",
			"internal/ui/finder.go:27:func NewFinderModel() *FinderModel {",
			"internal/ui/finder.go:65:func (f *FinderModel) handleKey(msg tea.KeyMsg) (Pane, tea.Cmd) {",
		},
		80, 24,
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
	)
	goldenTest(t, "grep_with_results", m.View())
}

func TestGoldenGrepManyResults(t *testing.T) {
	var items []string
	for i := 0; i < 40; i++ {
		items = append(items, fmt.Sprintf("internal/service/handler_%02d.go:%d:func Handle%02d() error {", i, i*10+5, i))
	}
	m := setupGrepModel("Handle", items, 80, 24, "package service\n\nfunc Handle00() error {\n\treturn nil\n}\n")
	goldenTest(t, "grep_many_results", m.View())
}

func TestGoldenGrepManyResultsScrolled(t *testing.T) {
	var items []string
	for i := 0; i < 40; i++ {
		items = append(items, fmt.Sprintf("internal/service/handler_%02d.go:%d:func Handle%02d() error {", i, i*10+5, i))
	}
	m := setupGrepModel("Handle", items, 80, 24, "")

	// カーソルを25行目まで移動
	var cur tea.Model = m
	for i := 0; i < 25; i++ {
		cur, _ = cur.Update(specialKeyMsg(tea.KeyDown))
	}
	got := cur.(Model)
	got.previewContent = "package service\n"
	goldenTest(t, "grep_many_results_scrolled", got.View())
}

func TestGoldenGrepWindowSmall(t *testing.T) {
	items := []string{
		"main.go:5:func main() {",
		"internal/ui/model.go:73:func NewModel() Model {",
		"internal/ui/finder.go:27:func NewFinderModel() *FinderModel {",
	}
	m := setupGrepModel("func", items, 40, 12, "package main\n\nfunc main() {\n}\n")
	goldenTest(t, "grep_window_small", m.View())
}

func TestGoldenGrepWindowLarge(t *testing.T) {
	var items []string
	for i := 0; i < 20; i++ {
		items = append(items, fmt.Sprintf("internal/pkg/module_%02d.go:%d:func Process%02d(ctx context.Context, req *Request) (*Response, error) {", i, i*15+3, i))
	}
	var previewLines []string
	for i := 1; i <= 40; i++ {
		previewLines = append(previewLines, fmt.Sprintf("// line %d", i))
	}
	m := setupGrepModel("Process", items, 160, 50, strings.Join(previewLines, "\n"))
	goldenTest(t, "grep_window_large", m.View())
}

func TestGoldenGrepLongPreview(t *testing.T) {
	items := []string{"main.go:1:package main"}
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d: some content here for testing purposes", i))
	}
	m := setupGrepModel("package", items, 80, 24, strings.Join(lines, "\n"))
	goldenTest(t, "grep_preview_long", m.View())
}

func TestGoldenGrepError(t *testing.T) {
	m := NewModel()
	m.mode = ModeGrep
	m.grepPane.loading = false
	m.grepPane.err = fmt.Errorf("rg: command not found")
	m.width = 80
	m.height = 24
	goldenTest(t, "grep_error", m.View())
}

func TestGoldenLoading(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24
	goldenTest(t, "loading", m.View())
}

func TestGoldenError(t *testing.T) {
	m := NewModel()
	m.finderPane.loading = false
	m.finderPane.err = fmt.Errorf("fd: command not found")
	m.width = 80
	m.height = 24
	goldenTest(t, "error", m.View())
}

// --- ゴールデンテストの行幅検証 ---

func TestGoldenViewLinesWithinWidth(t *testing.T) {
	sizes := []struct {
		name   string
		width  int
		height int
		count  int
	}{
		{"small", 40, 12, 5},
		{"medium", 80, 24, 20},
		{"large", 160, 50, 50},
	}

	t.Run("finder", func(t *testing.T) {
		for _, s := range sizes {
			t.Run(s.name, func(t *testing.T) {
				var files []string
				for i := 0; i < s.count; i++ {
					files = append(files, fmt.Sprintf("internal/pkg/very_long_package_name/module_%02d.go", i))
				}
				m := setupModel(files, s.width, s.height, strings.Repeat("x", 200)+"\n")
				assertViewWithinWidth(t, m.View(), s.width)
			})
		}
	})

	t.Run("grep", func(t *testing.T) {
		for _, s := range sizes {
			t.Run(s.name, func(t *testing.T) {
				var items []string
				for i := 0; i < s.count; i++ {
					items = append(items, fmt.Sprintf("internal/pkg/very_long_package_name/module_%02d.go:%d:func VeryLongFunctionNameForTesting%02d() error {", i, i*10, i))
				}
				m := setupGrepModel("VeryLong", items, s.width, s.height, strings.Repeat("x", 200)+"\n")
				assertViewWithinWidth(t, m.View(), s.width)
			})
		}
	})
}

// assertViewWithinWidth は View 出力の全行が指定幅以内であることを検証する。
func assertViewWithinWidth(t *testing.T, view string, maxWidth int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, maxWidth,
			"行 %d が幅を超えている（visual width=%d, max=%d）", i, w, maxWidth)
	}
}
