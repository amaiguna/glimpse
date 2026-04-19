# Issue #002: Pane.Update の戻り値が捨てられている

## 問題

親 Model の `delegateToPane` で子ペインの `Update` の戻り値 `Pane` を捨てている。

```go
func (m Model) delegateToPane(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    if m.mode == ModeGrep {
        _, cmd = m.grepPane.Update(msg)  // Pane を捨てている
    } else {
        _, cmd = m.finderPane.Update(msg)
    }
```

現在はポインタレシーバなので「たまたま動いている」が、Elm Architecture の原則に反している。
子 Model が値型に変わった場合や、`Update` 内で新しいインスタンスを返すような変更をしたら即壊れる。

## 対応方針

戻り値を受け取り、親 Model のフィールドに代入する。

```go
func (m Model) delegateToPane(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    if m.mode == ModeGrep {
        pane, cmd := m.grepPane.Update(msg)
        m.grepPane = pane.(*GrepModel)
    } else {
        pane, cmd := m.finderPane.Update(msg)
        m.finderPane = pane.(*FinderModel)
    }
```

## 優先度

高 — 将来のリファクタで即壊れるリスク。
