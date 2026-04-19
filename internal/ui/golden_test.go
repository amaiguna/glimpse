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
	m := NewModel()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	got := m2.(Model)
	got.mode = ModeGrep
	got.grepPane.textInput.SetValue("func")
	got.grepPane.items = []string{
		"main.go:5:func main() {",
		"internal/ui/model.go:73:func NewModel() Model {",
		"internal/ui/model.go:95:func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {",
		"internal/ui/finder.go:27:func NewFinderModel() *FinderModel {",
		"internal/ui/finder.go:65:func (f *FinderModel) handleKey(msg tea.KeyMsg) (Pane, tea.Cmd) {",
	}
	got.previewContent = "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	goldenTest(t, "grep_with_results", got.View())
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
	tests := []struct {
		name   string
		width  int
		height int
		files  int
	}{
		{"small", 40, 12, 5},
		{"medium", 80, 24, 20},
		{"large", 160, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files []string
			for i := 0; i < tt.files; i++ {
				files = append(files, fmt.Sprintf("internal/pkg/very_long_package_name/module_%02d.go", i))
			}
			m := setupModel(files, tt.width, tt.height, strings.Repeat("x", 200)+"\n")
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				w := ansi.StringWidth(line)
				assert.LessOrEqual(t, w, tt.width,
					"行 %d が幅を超えている（visual width=%d, max=%d）", i, w, tt.width)
			}
		})
	}
}
