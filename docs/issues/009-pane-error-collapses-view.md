# Issue #009: pane.Err() が真の時に View を全崩壊させる UX

**Status:** Closed (2026-04-26) — `internal/ui/model.go` の View 早期 return を廃止。`pane.Err()` non-nil 時は `errorLine` として header 直下に挿入し、リスト/プレビューの通常レイアウトを維持する方式に変更。golden (`error.txt`, `grep_error.txt`) を更新、`TestViewKeepsLayoutOnPaneError` で finder/grep 両モードでの不変条件を pin。

## 現状

`internal/ui/model.go:310-313`:

```go
// エラー表示（枠線なしで早期リターン）
if pane.Err() != nil {
    return header + "\n" + errorStyle.Render(fmt.Sprintf("error: %s", pane.Err().Error())) + "\n"
}
```

`pane.Err()` が non-nil になった瞬間、View は header と error 行の 2 行だけを返す。textinput / クエリプロンプト / 結果リスト / preview ペインなど通常レイアウト全体が消える。

### 観測される問題

- **入力途中の一時エラーで textinput が消える**: #007 のように broken regex を入力中、まさにユーザーがクエリを修正したい瞬間に何も見えない。バックスペースは効くが目隠しでの入力になる
- **recoverable / fatal が同一扱い**: regex 半完成（recoverable）も rg バイナリ未検出（fatal）も同じ「View 全崩壊」になる
- **直前の検索結果が捨てられる**: 一文字ミスタイプで結果リストが消え、次のクエリ成功で復帰する。視認性に乏しい

### 設計意図の確認

`// エラー表示（枠線なしで早期リターン）` のコメントから、レイアウト計算中に枠線描画でエラーメッセージが切れるのを避けたかった意図が読み取れる。ただし「メッセージを 1 行で出す」と「textinput を含む通常 UI を維持する」は両立可能で、現状の早期 return は過剰。

## 対応方針の選択肢

### A. ステータス行スタイル（早期 return 廃止）

通常レイアウトを維持し、textinput の直下（あるいは header 下）に 1 行だけ赤字で error メッセージを表示する。fatal とそうでないものを区別しないシンプル化。

```go
// 早期 return を廃止
errorLine := ""
if e := pane.Err(); e != nil {
    errorLine = errorStyle.Render(fmt.Sprintf("error: %s", e.Error())) + "\n"
}
// 通常レイアウトを組み立てた後に挿入
return header + "\n" + errorLine + body
```

### B. severity 区別

`pane.ErrSeverity()` 等の追加メソッドで `Recoverable` / `Fatal` を返し、Recoverable はステータス行、Fatal は早期 return（現状）を維持。区別ロジックがペインごとに分散する。

### C. severity + 専用フィールド分離

`pane.RecoverableErr()` と `pane.FatalErr()` を別メソッドとし、両者を別経路で表示。型システムレベルで強制できるが API 表面が増える。

## 判断

**A を採用**を推奨。理由:

- TUI で「fatal なので何も表示しない」は実害がほぼない。ユーザーは閉じるしかなく、その間に**何が起きたか視覚的に確認したい**ので通常レイアウト維持の方が良い
- 区別ロジック (B / C) は線引きが恣意的になりやすく、将来的に C に集約される可能性が高い
- 現状の早期 return が解決していた「枠線描画との衝突」は、`errorLine` を本体の前後に挿入すれば回避可能

実装範囲:

- `model.go:310-313` の早期 return を削除し、`errorLine` をレイアウト組み立て後に挿入
- 既存の golden test（`error.txt`、`grep_error.txt`）を更新（通常レイアウト + error 行に変わる）
- 不変条件テストとして「`pane.Err() != nil` でも textinput が含まれる」を追加

## 関連 issue

- #007 grep regex 入力時 UI 崩壊（本 issue の症状）
- #008 stderr 喪失（メッセージ品質改善との合わせ技で初めて UX が完成する）
- #010 エディタ起動失敗の黙殺（A 採用後はステータス行に出せる）

## 優先度

**高** — #007 修正の前提条件。単独でも UX 改善効果が大きい。
