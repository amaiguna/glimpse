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

## データフロー

### File Finder モード

```
fd / rg --files
  → []string (ファイルパス一覧)
    → fuzzy.Find(query, items)
      → []fuzzy.Match (スコア順)
        → UI リスト表示
          → 選択 → $EDITOR で開く
```

### Live Grep モード

```
キー入力 (デバウンス付き)
  → rg --json <pattern>
    → ParseRgJSON() → []Match{File, Line, Text}
      → UI リスト表示
        → 選択 → $EDITOR +行番号 ファイル
```

### Preview

```
カーソル移動
  → preview.ReadFile(selectedPath)
    → chroma でシンタックスハイライト
      → 右ペインに描画
```

## Bubbletea Elm Architecture

UI 層は Elm Architecture に従う。副作用は全て `Cmd` として返し、Model 自体は純粋な状態遷移のみ行う。

```
     Msg (キー入力, ウィンドウリサイズ, 外部プロセス完了)
      │
      ▼
  Update(msg) → (Model, Cmd)
      │              │
      ▼              ▼
   新しい状態      副作用の実行
      │          (rg 起動, ファイル読み込み等)
      ▼
   View() → string (ターミナル出力)
```

これにより Model テストでは Msg を投入して返却された Model の状態を検証でき、副作用 (Cmd) も返り値として取得できる。

## モード管理

```
Model.mode: ModeFinder | ModeGrep

切替: Tab キー (予定)

ModeFinder: finder パッケージを使用
ModeGrep:   grep パッケージを使用
preview:    両モード共通で右ペインに表示
```

## 外部コマンド依存

| コマンド | 用途 | フォールバック |
|---------|------|-------------|
| `fd --type f` | ファイル一覧 | `rg --files` |
| `rg --json <pattern>` | Live Grep | なし（必須） |
| `$EDITOR` | ファイルを開く | なし |

## テスト方針

### ファイル配置

テストは対象コードと同じディレクトリに `_test.go` で配置する。

```
internal/grep/
  grep.go
  grep_test.go              ← unit test
  grep_integration_test.go  ← integration test (ビルドタグ付き)
```

### テスト種別

| 対象 | 手法 | パッケージ |
|------|------|-----------|
| ファジーマッチ、rg パーサー | Table-Driven + Fuzz | finder, grep |
| Model 状態遷移 | Msg → Model 検証 | ui |
| View() 出力 | Golden Test (teatest) | ui |
| fd/rg 実行を伴うテスト | Integration Test | finder, grep |

### Integration Test の分離

`//go:build integration` ビルドタグで分離する。

```bash
go test ./...                      # unit のみ
go test -tags=integration ./...    # integration も含む
```

Integration Test は外部コマンド（fd, rg）の実行を伴うテストが対象。
