package ui

import (
	"strings"

	"github.com/amaiguna/telescope-tui/internal/finder"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// FinderModel はファイルファインダーモードのペイン。
// fd/rg --files で取得したファイル一覧をファジーフィルタリングする。
type FinderModel struct {
	textInput textinput.Model
	items     []string // フィルタ後の表示アイテム
	allFiles  []string // フィルタ前の全ファイルリスト
	cursor    int
	loading   bool
	err       error
}

// NewFinderModel は FinderModel を初期化して返す。
func NewFinderModel() *FinderModel {
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.Focus()
	ti.CharLimit = 256
	return &FinderModel{
		textInput: ti,
		loading:   true,
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
		return f, nil
	case tea.KeyDown:
		if len(f.items) > 0 && f.cursor < len(f.items)-1 {
			f.cursor++
		}
		return f, nil
	default:
		// テキスト入力は textinput に委譲
		prevQuery := f.textInput.Value()
		var cmd tea.Cmd
		f.textInput, cmd = f.textInput.Update(msg)
		if f.textInput.Value() != prevQuery {
			f.applyFilter()
		}
		return f, cmd
	}
}

// applyFilter はファジーフィルタを適用しカーソルをクランプする。
func (f *FinderModel) applyFilter() {
	query := f.textInput.Value()
	filtered := finder.FuzzyFilter(query, f.allFiles)
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

// View はリスト部分のみを描画する（ヘッダーは親 Model が担当）。
func (f *FinderModel) View() string {
	var b strings.Builder
	for i, item := range f.items {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == f.cursor {
			b.WriteString(selectedItemStyle.Render("> " + item))
		} else {
			b.WriteString(normalItemStyle.Render("  " + item))
		}
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

func (f *FinderModel) Query() string   { return f.textInput.Value() }
func (f *FinderModel) IsLoading() bool { return f.loading }
func (f *FinderModel) Err() error      { return f.err }

// TextInput は入力欄の View を返す（親 Model がヘッダーに組み込む用）。
func (f *FinderModel) TextInput() textinput.Model { return f.textInput }

// Reset はモード切替時にペインの状態をリセットする。
func (f *FinderModel) Reset() {
	f.textInput.SetValue("")
	f.cursor = 0
	f.err = nil
	f.items = f.allFiles
}

// Focus はテキスト入力にフォーカスを当てる。
func (f *FinderModel) Focus() tea.Cmd {
	return f.textInput.Focus()
}

// Blur はテキスト入力のフォーカスを外す。
func (f *FinderModel) Blur() {
	f.textInput.Blur()
}
