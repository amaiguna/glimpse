# Issue #005: View() 内のモード分岐を Pane インターフェースに統一する

## 問題

`View()` でヘッダーの TextInput 表示やエディタ起動時に、親 Model がペインの具象型を直接参照している。

```go
// View() 内
if m.mode == ModeGrep {
    inputView = m.grepPane.TextInput().View()
} else {
    inputView = m.finderPane.TextInput().View()
}

// handleEnter 内
case ModeGrep:
    file, line := parseGrepItem(selected)
```

`Pane` インターフェースを通じてアクセスすべき情報が漏れている。

## 対応方針

`Pane` インターフェースに以下を追加:

```go
TextInputView() string       // ヘッダー用テキスト入力の View
OpenTarget() (file string, line int)  // エディタで開く対象
```

これにより `View()` と `handleEnter` からモード分岐が消える。

```go
// View()
inputView = pane.TextInputView()

// handleEnter
file, line := pane.OpenTarget()
return m, openEditorCmd(file, line)
```

## 優先度

低 — 可読性の改善。機能的な問題はない。
