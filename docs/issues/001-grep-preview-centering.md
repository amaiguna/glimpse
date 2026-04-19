# Issue #001: Grep モードのプレビューをヒット行中心に表示する

## 問題

Grep モードで同一ファイルに複数ヒットがある場合、プレビューは常に1行目から表示される。
そのため、ヒット行が表示範囲外にあるとハイライトが見えない。

### 再現手順

1. 大きなファイル（100行以上）を含むプロジェクトで `go run .`
2. Tab で Grep モードに切り替え
3. 80行目付近にヒットするパターンを入力
4. → プレビューは1行目から表示され、ハイライト行が見えない

## 期待する動作

- Grep ヒット行がプレビューペインの**中央付近**に表示される
- ヒット行が先頭付近（1行目、2行目など）の場合は、上に余白を作らず先頭から表示する
- Finder モードの動作は変更しない

## 設計方針

### 変更対象

| ファイル | 変更内容 |
|---|---|
| `internal/preview/preview.go` | `ReadFileRange(path, startLine, maxLines)` を新設 |
| `internal/ui/pane.go` | `PreviewRange(visibleHeight int) int` をインターフェースに追加 |
| `internal/ui/finder.go` | `PreviewRange` 実装 — 常に `1` を返す |
| `internal/ui/grep_model.go` | `PreviewRange` 実装 + `DecoratePreview` の行番号調整 |
| `internal/ui/model.go` | `updatePreview()` で `PreviewRange` + `ReadFileRange` を使用 |

### 詳細

#### 1. `preview.ReadFileRange`

```go
func ReadFileRange(path string, startLine, maxLines int) (string, error)
```

- `startLine` は 1-based
- `startLine` 以前の行はスキップし、`maxLines` 行分を返す
- 既存の `ReadFile` はそのまま残す

#### 2. `Pane.PreviewRange`

```go
PreviewRange(visibleHeight int) int  // 表示開始行（1-based）を返す
```

- **FinderModel**: 常に `1`
- **GrepModel**: `max(1, hitLine - visibleHeight/2)`
  - `hitLine` は `parseGrepItem(selectedItem)` から取得
  - 先頭付近のクランプにより余白は生まれない

#### 3. `updatePreview` の変更

```go
startLine := pane.PreviewRange(m.contentHeight())
content, err := preview.ReadFileRange(filePath, startLine, m.contentHeight())
```

- `previewMaxLines` 定数の代わりに `contentHeight()` を使う

#### 4. `GrepModel.DecoratePreview` の調整

ハイライト対象の行番号をウィンドウ開始行からの相対位置に変換する:

```
表示上の行インデックス = hitLine - startLine
```

`GrepModel` が `PreviewRange` で返した `startLine` を内部に保持しておき、
`DecoratePreview` でオフセット計算に使う。

## テスト計画

- `preview.ReadFileRange` のユニットテスト（通常、先頭、末尾、範囲外）
- `GrepModel.PreviewRange` のテスト（中央配置、先頭クランプ）
- ゴールデンテスト更新（Grep 系）
- 行幅検証テスト（既存の `TestGoldenViewLinesWithinWidth`）が引き続き通ること
