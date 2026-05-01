# Issue #006: Pane インターフェースの肥大化

**Status:** Closed (2026-05-01)

**解消サマリ**: Pane を 4 ロールに分割（Pane / HeaderRenderer / Selector / PreviewDecorator）。本体は 6 メソッドに削減。Model.View() は HeaderRenderer / Selector / PreviewDecorator を type assertion で取得し、未実装ペインは安全にフォールバックする。Filtered Grep (proposal #001) Phase 1 の前提条件を満たす。

## 現状

`Pane` インターフェースは現在 11 メソッド。
Issue #001〜#005 の対応で `TextInputView`、`OpenTarget`、`PreviewRange` を追加した結果。

```go
type Pane interface {
    Update(msg tea.Msg) (Pane, tea.Cmd)
    View() string
    SelectedItem() string
    FilePath() string
    Query() string
    IsLoading() bool
    Err() error
    TextInputView() string
    OpenTarget() (file string, line int)
    PreviewRange(visibleHeight int) int
    DecoratePreview(content string, width int) string
}
```

`DecoratePreview` / `PreviewRange` は Finder 側がパススルー実装（そのまま返す / 常に 1）であり、
インターフェース分離の原則（ISP）の観点では分割候補。

## 対応方針の選択肢

### A. ロール別インターフェース分割

```go
type Pane interface {
    Update(msg tea.Msg) (Pane, tea.Cmd)
    View() string
    Query() string
    IsLoading() bool
    Err() error
    TextInputView() string
}

type Selector interface {
    SelectedItem() string
    FilePath() string
    OpenTarget() (file string, line int)
}

type PreviewDecorator interface {
    PreviewRange(visibleHeight int) int
    DecoratePreview(content string, width int) string
}
```

親 Model は型アサーションで必要なインターフェースを取得。
パススルー実装が不要になるが、型アサーションの管理コストが増える。

### B. 現状維持 + メソッド数の上限ルール

メソッド数が 12〜13 を超えたら分割を検討する。
現時点では 11 で、両ペインとも全メソッドを実装しており実害はない。

## 判断（初回）

現時点では **B（現状維持）** を採用。理由:

- 実装は2つ（Finder / Grep）のみで、パススルー実装のコストは低い
- 型アサーションを導入すると `View()` や `handleEnter()` のシンプルさが失われる
- 新しいモード（例: Buffer List）を追加するタイミングで再評価する

## 再評価 (2026-04-27)

[Proposal #001 (Filtered Grep)](../proposals/001-filtered-grep.md) で「複数入力欄を持つペイン」要件が確定。
`TextInputView() string` の単一返却前提が崩れ、include 用の query / filter 解析メソッドを追加で持たせると ISP 違反が更に深まる。
A 案へ切り替え、proposal #001 Phase 1 として実施することに決定。

## 実装方針 (確定)

最終形は A 案を採用しつつ `TextInputView` を `HeaderViews() []string` に置換（複数入力欄に備えた拡張）：

```go
// 親 Model が必ず使う基本契約 (6 メソッド)
type Pane interface {
    Update(tea.Msg) (Pane, tea.Cmd)
    View() string
    Query() string
    IsLoading() bool
    Err() error
    SetErr(error)
}

// ヘッダー描画用 (複数入力を許容)
type HeaderRenderer interface {
    HeaderViews() []string  // 各入力欄の View を返す (1 個 or 複数)
}

// 選択 / オープン
type Selector interface {
    SelectedItem() string
    FilePath() string
    OpenTarget() (file string, line int)
}

// プレビュー装飾
type PreviewDecorator interface {
    PreviewRange(visibleHeight int) int
    DecoratePreview(content string, width int) string
}
```

親 Model は `if sel, ok := pane.(Selector); ok { ... }` パターンでオプショナルロールを取得。
未実装ペインは機能が安全に「無効化」される（Selector なし → Enter no-op、PreviewDecorator なし → 装飾なし、HeaderRenderer なし → 入力欄空のヘッダー）。

## 実装結果 (2026-05-01)

- `internal/ui/pane.go`: 4 インターフェースに分割。Pane 本体は 6 メソッド (Update / View / Query / IsLoading / Err / SetErr)
- `internal/ui/finder.go`, `internal/ui/grep_model.go`: `TextInputView` → `HeaderViews() []string` に置換 (1 要素返却)
- `internal/ui/model.go`:
  - `handleEnter` / `previewCmd` / `View` を type assertion ベースに変更
  - 複数行ヘッダーに対応した `contentHeight` 計算 (`extraHeaderLines = len(headerLines) - 1` を引く)
  - 2 行目以降のヘッダーはモードラベル幅ぶん空白パディングして入力欄列を揃える
- `internal/ui/pane_interface_test.go`: 新規。compile-time 型アサーション + メソッド数 + HeaderViews 戻り値を pin
- 既存ゴールデン無変更（1 行ヘッダー時の出力は完全互換）。fuzz / vet / 全テスト通過

## 優先度

中 — [Proposal #001 (Filtered Grep)](../proposals/001-filtered-grep.md) の前提タスク。複数入力欄を持つペインへの拡張に備え、本機能着手前に分割設計を確定させる必要がある。

## 関連

- 前提となる proposal: [Proposal #001 Filtered Grep モードの追加](../proposals/001-filtered-grep.md) — 分割の方向性 (HeaderRenderer / Selector / PreviewDecorator) のたたき台あり
