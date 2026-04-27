# Issue #006: Pane インターフェースの肥大化

**Status:** Open

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

## 判断

現時点では **B（現状維持）** を採用。理由:

- 実装は2つ（Finder / Grep）のみで、パススルー実装のコストは低い
- 型アサーションを導入すると `View()` や `handleEnter()` のシンプルさが失われる
- 新しいモード（例: Buffer List）を追加するタイミングで再評価する

## 優先度

中 — [Proposal #001 (Filtered Grep)](../proposals/001-filtered-grep.md) の前提タスク。複数入力欄を持つペインへの拡張に備え、本機能着手前に分割設計を確定させる必要がある。

## 関連

- 前提となる proposal: [Proposal #001 Filtered Grep モードの追加](../proposals/001-filtered-grep.md) — 分割の方向性 (HeaderRenderer / Selector / PreviewDecorator) のたたき台あり
