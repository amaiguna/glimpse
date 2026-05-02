# アーキテクチャ

## 全体構成

```
main.go
  └─ internal/
       ├─ ui/          … Bubbletea Model（入力・状態管理・描画）
       ├─ finder/      … ファイル一覧取得 + ファジーフィルタ
       ├─ grep/        … rg --json 実行 + 出力パース
       └─ preview/     … ファイル読み込み + シンタックスハイライト
```

## レイヤー構成

```
┌──────────────────────────────────────────────┐
│  UI 層 (internal/ui)                         │
│  Bubbletea Elm Architecture                  │
│  Model ← Msg → Update → (Model, Cmd)        │
│  View() でターミナル描画                       │
├──────────────────────────────────────────────┤
│  ロジック層                                    │
│  ┌─────────────┬──────────────┬────────────┐ │
│  │ finder      │ grep         │ preview    │ │
│  │ ファイル列挙  │ rg JSON パース │ ファイル読込 │ │
│  │ ファジーマッチ │ 検索実行      │ ハイライト  │ │
│  └─────────────┴──────────────┴────────────┘ │
├──────────────────────────────────────────────┤
│  外部プロセス                                   │
│  fd / rg (ファイル列挙)    rg --json (検索)     │
└──────────────────────────────────────────────┘
```

## UI 層の設計

### Pane インターフェース

親 Model は `Pane` インターフェースを通じて Finder / Grep を統一的に扱う。
モード固有のロジック（プレビュー範囲、エディタ起動パラメータ、入力 View 等）は全て各ペインに閉じ込められ、親はインターフェース経由でのみアクセスする。

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

現在 11 メソッド。実装は `FinderModel` と `GrepModel` の2つ。

### Msg ルーティング

```
tea.Msg
  │
  ├─ tea.KeyMsg (グローバル)
  │    ├─ Ctrl+C / Esc  → tea.Quit
  │    ├─ Tab            → switchMode()
  │    ├─ Enter          → pane.OpenTarget() → openEditorCmd()
  │    └─ その他          → delegateToPane() (アクティブペイン)
  │
  ├─ paneMsg (ペイン固有) ← PaneTarget() で宛先を自動判別
  │    ├─ FilesLoadedMsg, FilesErrorMsg  → delegateToFinder()
  │    └─ GrepDoneMsg, GrepErrorMsg, debounceTickMsg → delegateToGrep()
  │
  ├─ PreviewLoadedMsg → previewContent にセット（パス照合）
  ├─ tea.WindowSizeMsg → リサイズ処理
  └─ default → delegateToPane() (アクティブペイン)
```

新しいペイン固有 Msg を追加する場合は、`PaneTarget() Mode` メソッドを実装するだけで `Update()` の変更は不要。

### プレビューの非同期読み込み

```
カーソル移動 / モード切替
  → previewCmd() が tea.Cmd を返す
    → 非同期で ReadFileRange + Highlight を実行
      → PreviewLoadedMsg として結果を受信
        → パス照合で古いプレビューの上書きを防止
          → previewContent にセット → View() で描画
```

- Grep モードでは `PreviewRange()` がヒット行を中央に配置する開始行を計算
- `DecoratePreview()` で検索クエリにマッチする単語のみハイライト（シンタックスハイライトの前景色を保持）

## データフロー

### File Finder モード

```
fd / rg --files
  → FilesLoadedMsg ([]string)
    → fuzzy.Find(query, items)
      → UI リスト表示
        → 選択 → $EDITOR で開く
```

### Live Grep モード

```
キー入力 (100ms デバウンス)
  → debounceTickMsg
    → rg --json <pattern>
      → GrepDoneMsg ([]grep.Match)
        → UI リスト表示（ファイル名のみ）
          → 選択 → $EDITOR +行番号 ファイル
```

### Preview

```
カーソル移動
  → previewCmd()
    → preview.ReadFileRange(path, startLine, height)
      → chroma でシンタックスハイライト
        → PreviewLoadedMsg
          → pane.DecoratePreview() でマッチ単語ハイライト
            → 右ペインに描画
```

## モード管理

```
Model.mode: ModeFinder | ModeGrep

切替: Tab キー
  → switchMode()
    → 現ペイン Blur + 新ペイン Reset + Focus
    → previewCmd() でプレビュー更新

ModeFinder: FinderModel (finder パッケージを使用)
ModeGrep:   GrepModel (grep パッケージを使用)
preview:    両モード共通で右ペインに表示
```

## 外部コマンド依存

| コマンド | 用途 | フォールバック |
|---------|------|-------------|
| `fd --type f` | ファイル一覧 | `rg --files` |
| `rg --json <pattern>` | Live Grep | なし（必須） |
| `$EDITOR` | ファイルを開く | `vim` |

## テスト方針

### ファイル配置

テストは対象コードと同じディレクトリに `_test.go` で配置する。

```
internal/ui/
  model.go
  model_test.go        ← Model テスト
  scenario_test.go     ← シナリオテスト（Msg 駆動）
  golden_test.go       ← ゴールデンテスト
  fuzz_test.go         ← Fuzz テスト（panic 検出）

internal/grep/
  grep.go
  grep_test.go         ← unit test + fuzz test
  grep_integration_test.go  ← integration test (ビルドタグ付き)
```

### テスト種別

| 対象 | 手法 | 目的 |
|------|------|------|
| ファジーマッチ、rg パーサー | Table-Driven + Fuzz | ロジック正確性 + 不正入力耐性 |
| Model 状態遷移 | Msg → Model 検証 | Update の正しさ |
| View() 出力 | Golden Test | レイアウト退行検知 |
| 動作シナリオ | Scenario Test (Msg 駆動) | リファクタリング安全網 |
| 異常操作 | Fuzz Test (ランダム Msg 列) | panic 防止 |
| fd/rg 実行 | Integration Test | 外部コマンド連携 |

### Integration Test の分離

`//go:build integration` ビルドタグで分離する。

```bash
go test ./...                      # unit のみ
go test -tags=integration ./...    # integration も含む
```
