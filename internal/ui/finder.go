package ui

import (
	"context"
	"strings"
	"time"

	"github.com/amaiguna/glimpse-tui/internal/finder"
	"github.com/amaiguna/glimpse-tui/internal/sanitize"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// finderListTimeout はファイル列挙（fd / rg --files）に許す最大時間。
// 大規模リポでも十分な余裕を取りつつ、暴走プロセスは kill されるようにする（M-3）。
const finderListTimeout = 30 * time.Second

// FinderModel はファイルファインダーモードのペイン。
// fd/rg --files で取得したファイル一覧をファジーフィルタリングする。
type FinderModel struct {
	textInput  textinput.Model
	items      []string // フィルタ後の表示アイテム
	allFiles   []string // フィルタ前の全ファイルリスト
	cursor     int
	offset     int // スクロールオフセット（表示先頭行）
	viewHeight int // 表示可能行数（親から設定）
	viewWidth  int // 表示可能幅（親から設定）
	loading    bool
	err        error
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
// finderListTimeout のタイムアウトを設定し、fd/rg プロセスが暴走した場合でも
// 一定時間で kill されるようにする（M-3）。
func loadFilesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), finderListTimeout)
		defer cancel()
		items, err := finder.ListFiles(ctx)
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
	case tea.KeyUp, tea.KeyCtrlP:
		if f.cursor > 0 {
			f.cursor--
			f.clampOffset()
		}
		return f, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if len(f.items) > 0 && f.cursor < len(f.items)-1 {
			f.cursor++
			f.clampOffset()
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

// clampOffset はカーソルが表示範囲内に収まるよう offset を調整する。
func (f *FinderModel) clampOffset() {
	// cursor を items 範囲内にクランプ
	if f.cursor < 0 {
		f.cursor = 0
	}
	if len(f.items) > 0 && f.cursor >= len(f.items) {
		f.cursor = len(f.items) - 1
	}
	h := f.visibleHeight()
	if h <= 0 {
		return
	}
	if f.cursor < f.offset {
		f.offset = f.cursor
	}
	if f.cursor >= f.offset+h {
		f.offset = f.cursor - h + 1
	}
}

// visibleHeight は表示可能行数を返す。
func (f *FinderModel) visibleHeight() int {
	if f.viewHeight > 0 {
		return f.viewHeight
	}
	return len(f.items)
}

// SetViewSize は親から表示可能な行数と幅を設定する。
func (f *FinderModel) SetViewSize(h, w int) {
	f.viewHeight = h
	f.viewWidth = w
	f.clampOffset()
}

// truncateToWidth は文字列を表示幅 w に切り詰める。
func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Truncate(s, w, "")
}

// View はリスト部分のみを描画する（ヘッダーは親 Model が担当）。
func (f *FinderModel) View() string {
	h := f.visibleHeight()
	if f.offset < 0 || f.offset > len(f.items) {
		f.offset = 0
	}
	end := f.offset + h
	if end > len(f.items) {
		end = len(f.items)
	}
	visible := f.items[f.offset:end]

	// カーソル記号 "> " の分を引いた残り幅
	itemWidth := f.viewWidth - 2

	var b strings.Builder
	for i, item := range visible {
		if i > 0 {
			b.WriteString("\n")
		}
		absIdx := f.offset + i
		// 描画用にサニタイズ。SelectedItem/FilePath/OpenTarget は raw のまま使うため
		// ここで保持される items 自体は変更しない。
		display := truncateToWidth(sanitize.ForTerminal(item), itemWidth)
		if absIdx == f.cursor {
			b.WriteString(selectedItemStyle.Render("> " + display))
		} else {
			b.WriteString(normalItemStyle.Render("  " + display))
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

// DecoratePreview はプレビューコンテンツをそのまま返す（Finder モードでは装飾なし）。
func (f *FinderModel) DecoratePreview(content string, width int) string {
	return content
}

// TextInput は入力欄のモデルを返す。
func (f *FinderModel) TextInput() textinput.Model { return f.textInput }

// TextInputView はヘッダー用テキスト入力の View 文字列を返す。
func (f *FinderModel) TextInputView() string { return f.textInput.View() }

// PreviewRange はプレビューの表示開始行を返す。Finder は常に先頭から。
func (f *FinderModel) PreviewRange(_ int) int { return 1 }

// OpenTarget はエディタで開く対象を返す。Finder は行番号なし（0）。
func (f *FinderModel) OpenTarget() (string, int) {
	selected := f.SelectedItem()
	return selected, 0
}

// Reset はモード切替時にペインの状態をリセットする。
func (f *FinderModel) Reset() {
	f.textInput.SetValue("")
	f.cursor = 0
	f.offset = 0
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
