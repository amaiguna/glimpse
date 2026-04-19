# Issue #003: Msg ルーティングの曖昧さ

## 問題

`FilesLoadedMsg` は Finder 専用、`GrepDoneMsg` / `GrepErrorMsg` / `debounceTickMsg` は Grep 専用だが、
親 Model の `Update` では `default` ケースで全て `delegateToPane` に流している。

```go
default:
    return m.delegateToPane(msg)
```

非アクティブなペインの Msg も、アクティブなペインに届いてしまう構造になっている。
例えば Grep モード中に `FilesLoadedMsg` が来ると GrepModel の `Update` に渡されて無視される。

壊れはしないが、設計意図として各 Msg は対応するペインに届くべき。

## 対応方針

親 Model の `Update` で Msg の型を見て適切なペインにルーティングする。

```go
case FilesLoadedMsg, FilesErrorMsg:
    _, cmd = m.finderPane.Update(msg)
case GrepDoneMsg, GrepErrorMsg, debounceTickMsg:
    _, cmd = m.grepPane.Update(msg)
```

## 優先度

中 — 実害は今のところないが、ペインが増えたときに問題になる。
