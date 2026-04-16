# telescope-tui (仮)

Neovim Telescope にインスパイアされたスタンドアローンの TUI ファジーファインダー。
エディタに依存せず、ターミナルから単体で動作する。

## MVP 機能

### File Finder

- `fd` または `rg --files` の出力をソースとしてファイル一覧を取得
- キー入力に応じたファジーフィルタリング
- 選択したファイルを `$EDITOR` で開いて終了

### Live Grep

- `rg --json` をデバウンス付きで実行
- ファイル名 + 行番号 + マッチ行を一覧表示
- 選択で `$EDITOR +行番号 ファイル` を起動

### Preview

- 右ペインにファイル内容をプレビュー表示
- シンタックスハイライト付き

## 技術スタック

### 言語・フレームワーク

| カテゴリ | 選定 |
| --- | --- |
| 言語 | Go |
| TUI フレームワーク | [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) (Elm Architecture) |
| レイアウト・スタイリング | [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) |
| UI コンポーネント | [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) |

### 主要ライブラリ

| 用途 | ライブラリ |
| --- | --- |
| ファジーマッチ | [sahilm/fuzzy](https://github.com/sahilm/fuzzy) |
| シンタックスハイライト | [alecthomas/chroma](https://github.com/alecthomas/chroma) |

### ランタイム依存

- [ripgrep (rg)](https://github.com/BurntSushi/ripgrep) — grep ソース、ファイル一覧のフォールバック
- [fd](https://github.com/sharkdp/fd) — ファイル一覧のソース

## アーキテクチャ方針

- Telescope の「source / picker」抽象化は初期段階では行わず、File Finder と Live Grep をハードコードで実装する
- 使い勝手を確認しながら後続で拡張・抽象化を進める

## テスト戦略

TDD ベースで開発し、以下の形式を組み合わせる。

### Table-Driven Test

- Go 標準の `testing` パッケージ + `testify/assert` をベースに使用
- テストケースを struct のスライスとして定義し、`t.Run` でサブテスト化
- ファジーマッチのスコアリング、`rg --json` 出力のパースなど入力バリエーションの多いロジックに適用

### Fuzz Test

- Go 1.18+ ネイティブの `testing.F` を使用
- 主なターゲット:
  - ファジーマッチ関数（パニック・想定外入力の検出）
  - rg 出力パーサー（不正な JSON、切り詰められた入力への耐性）

### Golden Test

- `charmbracelet/x/exp/teatest` を使用
- Model にメッセージを送り、`View()` の出力をゴールデンファイルと比較
- TUI のレイアウト崩れ・表示の退行を機械的に検知
- ゴールデンファイルの更新は `-update` フラグで再生成

### Bubbletea Model Test

- Elm Architecture の特性を活かし、Model に Msg を投入して返却された Model の状態を検証
- 副作用（Cmd）も返り値として取得し、「このキー入力で rg が呼ばれるか」等の検証を行う
- Table-Driven Test との相性が良く、入力 Msg と期待 Model 状態のペアをテーブル化してカバレッジを確保

### テスト構造の概観

```
ロジック層: Table-Driven Test + Fuzz Test
    ファジーマッチ、パーサー、フィルタリングロジック

UI 層: Golden Test + Model Test
    View() 出力の退行検知、Msg → Model 状態遷移の検証
```

## 開発環境

- OS: Arch Linux
- エディタ: Zed
