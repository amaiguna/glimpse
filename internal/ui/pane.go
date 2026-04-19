package ui

import tea "github.com/charmbracelet/bubbletea"

// paneMsg は特定のペインに宛てられた Msg が実装するインターフェース。
// 親 Model の Update はこのインターフェースで宛先を判別し、対応するペインにルーティングする。
type paneMsg interface {
	tea.Msg
	PaneTarget() Mode
}

// Pane はファインダーの各モード（Finder, Grep）が実装する共通インターフェース。
// 親 Model はアクティブな Pane にメッセージをディスパッチし、View を取得する。
type Pane interface {
	// Update はメッセージを処理し、更新された Pane と Cmd ���返す。
	Update(msg tea.Msg) (Pane, tea.Cmd)
	// View はペインの内容を文字列で返す。
	View() string
	// SelectedItem は現在カーソルが指しているアイテムを返す。
	// アイテムがない場合は空文字列を返す。
	SelectedItem() string
	// FilePath はプレビュー表示用のファイルパスを返す。
	// Grep モードでは "file:line:text" からファイルパスを抽出する。
	FilePath() string
	// Query は現在の検索クエリを返す。
	Query() string
	// IsLoading はデータ読み込み中かどうかを返す。
	IsLoading() bool
	// Err はエラーがあれば返す。
	Err() error
	// DecoratePreview はプレビューコンテンツにペイン固有の装飾を施す。
	// 幅 width は表示可能なカラム数。
	DecoratePreview(content string, width int) string
}
