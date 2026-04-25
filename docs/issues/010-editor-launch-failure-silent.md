# Issue #010: エディタ起動失敗が黙殺される

## 現状

`internal/ui/model.go:282-292`:

```go
func openEditorCmd(file string, line int) tea.Cmd {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "vim"
    }
    args := buildEditorArgs(editor, file, line)
    c := exec.Command(editor, args...)
    return tea.ExecProcess(c, func(err error) tea.Msg {
        return EditorFinishedMsg{Err: err}
    })
}
```

`tea.ExecProcess` は子プロセスに端末を完全に引き渡すため、glimpse 側からは stdout / stderr を捕捉できない。終了後に渡される `err` は `*exec.Error`（exec 自体の失敗）か `*exec.ExitError`（非0 終了）の form を取る。

しかし受け側 (`model.go:139-140`) は:

```go
case EditorFinishedMsg:
    return m, nil
```

`Err` を**完全に捨てている**。

### 観測される問題

- `EDITOR=nonexistent-editor` で起動 → exec 失敗するが何も表示されない
- `EDITOR` 未設定 + fallback `vim` がインストールされていない環境 → 同上
- エディタが segfault / 非0 終了 → 同上
- ユーザーは「あれ？開かない」状態になり原因不明

### 想定するケース

- `EDITOR` の path がタイポ
- 軽量コンテナや minimal 環境で `vim` 不在
- エディタ自体は起動したがファイルアクセス権限で失敗
- buildEditorArgs が想定外の組み立て結果を返したケース（防御的に）

## 対応方針の選択肢

### A. err を pane.err 経由で表示

```go
case EditorFinishedMsg:
    if msg.Err != nil {
        // pane に err を反映する setter を追加
        m.activePane().setErr(msg.Err)
    }
    return m, nil
```

#009 で View ステータス行が導入されると、`error: editor 'foo' not found` のように見える。setter は既存 `Err()` getter とペアで `Pane` インターフェースに加える形になる（インターフェース肥大化 — #006 の議論にも関わる）。

### B. エディタ起動前に LookPath で事前検証

`openEditorCmd` の冒頭で `exec.LookPath(editor)` を呼び、見つからなければその時点でエラー化（pane に反映）して `tea.ExecProcess` を試みない。

```go
if _, err := exec.LookPath(editor); err != nil {
    // pane に err を反映してから空 Cmd を返す
    return func() tea.Msg { return EditorFinishedMsg{Err: err} }
}
```

事前検証で大半の「起動できない」を捕捉できる。ただし、起動後のクラッシュは捕捉できない。

### C. A + B 両方

事前検証で「即時に分かる失敗」を弾き、それでも漏れた exec 失敗は事後的にステータス行へ。最も robust。

## 判断

**C を採用**を推奨。実装コストは小さく、デバッグ性に大きく寄与する。`Pane.setErr` 相当の機構は #009 の修正と合わせて導入するのが自然。

実装の指針:

1. `Pane` インターフェースに `SetErr(error)` を追加（#006 のメソッド数増加には目を瞑る）
2. `openEditorCmd` 入口で `exec.LookPath` を呼び、失敗時はその場で `EditorFinishedMsg{Err: err}` を発火する Cmd を返す
3. `EditorFinishedMsg` 受信側で `msg.Err != nil` なら active pane に SetErr

## 関連 issue

- #006 Pane インターフェースの肥大化（SetErr 追加で再評価ライン 12 に到達）
- #008 stderr 喪失（同種テーマだが `tea.ExecProcess` は端末引き継ぎで仕組みが異なるため独立）
- #009 pane.Err() 時の View 動作（A 採用後の表示先となるステータス行はこの issue で導入される）

## 優先度

**中** — 頻度は高くないが、起きた時の「何も起きない」体験は TUI として致命的。`$EDITOR` 未設定の minimal 環境でいきなり遭遇する典型シナリオがある。
