package ui

import tea "github.com/charmbracelet/bubbletea"

// paneMsg は特定のペインに宛てられた Msg が実装するインターフェース。
// 親 Model の Update はこのインターフェースで宛先を判別し、対応するペインにルーティングする。
type paneMsg interface {
	tea.Msg
	PaneTarget() Mode
}

// Pane はファインダーの各モードが実装する最小限の共通契約（#006）。
// Update / View / クエリ / ローディング / エラーのコア状態のみを表現し、
// 「選択」「ヘッダー描画」「プレビュー装飾」など任意ロールは別インターフェースに分離する。
// 親 Model はこの最小契約に依存し、追加ロールが要るときだけ type assertion で取得する。
type Pane interface {
	// Update はメッセージを処理し、更新された Pane と Cmd を返す。
	Update(msg tea.Msg) (Pane, tea.Cmd)
	// View はペインの内容を文字列で返す。
	View() string
	// Query は現在の検索クエリを返す。
	Query() string
	// IsLoading はデータ読み込み中かどうかを返す。
	IsLoading() bool
	// Err はエラーがあれば返す。
	Err() error
	// SetErr はペインに表示すべきエラーをセットする（#010）。
	// エディタ起動失敗のように Pane 外部で生じた非同期エラーを active pane の
	// ステータス行に表面化する経路として使う。
	SetErr(err error)
}

// HeaderRenderer はヘッダー入力欄を描画するロール（#006 / proposal #001）。
// Filtered Grep のように複数入力欄を持つペインに備え、文字列スライスで返す。
// 単一入力欄のペインは要素 1 のスライスを返す。
type HeaderRenderer interface {
	// HeaderViews は各入力欄の View 文字列をスライスで返す。
	// 親 Model はこれを縦に並べてヘッダー領域を描画する。
	HeaderViews() []string
}

// Selector は「行を選んでファイルを開く」ロール（#006）。
// プレビュー対象の特定とエディタ起動先の決定に必要なメソッドをまとめる。
// Buffer List のような将来モードがこのロールを持たない可能性を許容する。
type Selector interface {
	// SelectedItem は現在カーソルが指しているアイテムを返す。
	// アイテムがない場合は空文字列を返す。
	SelectedItem() string
	// FilePath はプレビュー表示用のファイルパスを返す。
	// Grep モードでは "file:line:text" からファイルパスを抽出する。
	FilePath() string
	// OpenTarget はエディタで開く対象のファイルパスと行番号を返す。
	// 選択アイテムがない場合は空文字列と 0 を返す。
	OpenTarget() (file string, line int)
}

// PreviewDecorator はプレビューペインに固有の装飾を施すロール（#006）。
// Finder のようにパススルーで済む実装はこのロールを持たなくてもよい設計とし、
// 親 Model 側で type assertion 失敗時は無装飾フォールバックする。
type PreviewDecorator interface {
	// PreviewRange はプレビューの表示開始行（1-based）を返す。
	// visibleHeight は表示可能な行数。ヒット行を中央に配置するために使う。
	PreviewRange(visibleHeight int) int
	// DecoratePreview はプレビューコンテンツにペイン固有の装飾を施す。
	// 幅 width は表示可能なカラム数。
	DecoratePreview(content string, width int) string
}
