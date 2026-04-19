# Issue #006: Pane インターフェースの肥大化

## 問題

現在 `Pane` インターフェースに 8 メソッドがある。
`DecoratePreview` のように片方のペインしか実質的な実装を持たないメソッドが増えると、
インターフェース分離の原則（ISP）に反する。

Issue #001 で `PreviewRange` を追加するとさらに増加する。

```go
type Pane interface {
    Update(msg tea.Msg) (Pane, tea.Cmd)
    View() string
    SelectedItem() string
    FilePath() string
    Query() string
    IsLoading() bool
    Err() error
    DecoratePreview(content string, width int) string
    // + PreviewRange? TextInputView? OpenTarget?
}
```

## 対応方針の選択肢

### A. ロール別インターフェース分割

```go
type Pane interface {
    Update(msg tea.Msg) (Pane, tea.Cmd)
    View() string
    Query() string
    IsLoading() bool
    Err() error
}

type Selector interface {
    SelectedItem() string
    FilePath() string
}

type PreviewDecorator interface {
    DecoratePreview(content string, width int) string
}
```

親 Model は型アサーションで必要なインターフェースを取得。

### B. 現状維持 + メソッド数の上限ルール

メソッド数が 10 を超えたら分割を検討する。
現時点では 8 で許容範囲内。

## 優先度

低 — 現時点では許容範囲。メソッド追加のタイミングで再評価する。
