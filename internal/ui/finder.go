package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/amaiguna/telescope-tui/internal/finder"
)

// FinderModel はファイルファインダーモードのペイン。
// fd/rg --files で取得したファイル一覧をファジーフィルタリングする。
type FinderModel struct {
	query    string
	items    []string // フィルタ後の表示アイテム
	allFiles []string // フィルタ前の全ファイルリスト
	cursor   int
	loading  bool
	err      error
}

// NewFinderModel は FinderModel を初期化して返す。
func NewFinderModel() *FinderModel {
	return &FinderModel{
		loading: true,
	}
}

// loadFilesCmd はファイル列挙を非同期で実行するコマンドを返す。
func loadFilesCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := finder.ListFiles()
		if err != nil {
			return FilesErrorMsg{Err: err}
		}
		return FilesLoadedMsg{Items: items}
	}
}

func (f *FinderModel) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return f.handleKey(msg)
	case FilesLoadedMsg:
		f.loading = false
		f.err = nil
		f.allFiles = msg.Items
		f.applyFilter()
	case FilesErrorMsg:
		f.loading = false
		f.err = msg.Err
	}
	return f, nil
}

func (f *FinderModel) handleKey(msg tea.KeyMsg) (Pane, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if f.cursor > 0 {
			f.cursor--
		}
	case tea.KeyDown:
		if len(f.items) > 0 && f.cursor < len(f.items)-1 {
			f.cursor++
		}
	case tea.KeyBackspace:
		if len(f.query) > 0 {
			f.query = f.query[:len(f.query)-1]
			f.applyFilter()
		}
	case tea.KeyRunes:
		f.query += string(msg.Runes)
		f.applyFilter()
	}
	return f, nil
}

// applyFilter はファジーフィルタを適用しカーソルをクランプする。
func (f *FinderModel) applyFilter() {
	filtered := finder.FuzzyFilter(f.query, f.allFiles)
	if filtered == nil {
		f.items = nil
	} else {
		f.items = make([]string, len(filtered))
		for i, v := range filtered {
			f.items[i] = v.Str
		}
	}
	if len(f.items) == 0 {
		f.cursor = 0
	} else if f.cursor >= len(f.items) {
		f.cursor = len(f.items) - 1
	}
}

func (f *FinderModel) View() string {
	var b strings.Builder
	for i, item := range f.items {
		if i > 0 {
			b.WriteString("\n")
		}
		cursor := "  "
		if i == f.cursor {
			cursor = "> "
		}
		b.WriteString(cursor + item)
	}
	return b.String()
}

func (f *FinderModel) SelectedItem() string {
	if len(f.items) == 0 {
		return ""
	}
	return f.items[f.cursor]
}

// FilePath はプレビュー用のファイルパスを返す。Finder モードではアイテムがそのままパス。
func (f *FinderModel) FilePath() string {
	return f.SelectedItem()
}

func (f *FinderModel) Query() string   { return f.query }
func (f *FinderModel) IsLoading() bool { return f.loading }
func (f *FinderModel) Err() error      { return f.err }

// Reset はモード切替時にペインの状態をリセットする。
func (f *FinderModel) Reset() {
	f.query = ""
	f.cursor = 0
	f.err = nil
	f.items = f.allFiles
}
