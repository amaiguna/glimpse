# Issue #004: updatePreview の同期I/Oがブロッキングになる

## 問題

`delegateToPane` の中で `m.updatePreview()` を呼んでいるが、これは同期的なファイルI/O。

```go
func (m Model) delegateToPane(msg tea.Msg) (tea.Model, tea.Cmd) {
    // ...ペイン更新...
    m.updatePreview()  // ← preview.ReadFile + preview.Highlight が同期実行
    return m, cmd
}
```

Elm Architecture では副作用は `Cmd` として返すのが原則。
ファイルが大きい場合やネットワークストレージの場合、UIスレッドがブロッキングされる。

## 対応方針

プレビュー読み込みを `Cmd` 化する。

1. `updatePreview` は `tea.Cmd` を返す関数に変更
2. プレビュー結果は `PreviewLoadedMsg` として非同期に Model に返す
3. `View()` は `m.previewContent` がセットされるまでローディング表示

```go
type PreviewLoadedMsg struct {
    Content string
    Path    string  // 古いプレビューが上書きされないようパスを照合
}
```

## 優先度

中 — 現在のプロジェクト規模では実害は少ないが、大ファイルで顕在化する。
