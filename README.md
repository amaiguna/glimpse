# telescope-tui

Neovim Telescope にインスパイアされたスタンドアローンの TUI ファジーファインダー。
エディタに依存せず、ターミナルから単体で動作する。

## 機能

### File Finder

- `fd` または `rg --files` の出力をソースとしてファイル一覧を取得
- キー入力に応じたファジーフィルタリング
- 選択したファイルを `$EDITOR` で開いて終了

### Live Grep

- `rg --json` をデバウンス付きで実行
- ファイル名一覧を表示
- 選択で `$EDITOR +行番号 ファイル` を起動

### Preview

- 右ペインにファイル内容をプレビュー表示（シンタックスハイライト付き）
- Grep モードではヒット行をプレビュー中央に配置
- マッチ単語のみをハイライト（シンタックスハイライトの色を保持）

### 操作

| キー | 動作 |
| --- | --- |
| 文字入力 | フィルタリング / 検索パターン |
| `↑` / `Ctrl+P` | カーソル上移動 |
| `↓` / `Ctrl+N` | カーソル下移動 |
| `Tab` | Finder ↔ Grep モード切替 |
| `Enter` | 選択アイテムを `$EDITOR` で開く |
| `Esc` / `Ctrl+C` | 終了 |

## 技術スタック

### 言語・フレームワーク

| カテゴリ | 選定 |
| --- | --- |
| 言語 | Go |
| TUI フレームワーク | [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) (Elm Architecture) |
| レイアウト・スタイリング | [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) |
| UI コンポーネント | [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) (textinput) |

### 主要ライブラリ

| 用途 | ライブラリ |
| --- | --- |
| ファジーマッチ | [sahilm/fuzzy](https://github.com/sahilm/fuzzy) |
| シンタックスハイライト | [alecthomas/chroma](https://github.com/alecthomas/chroma) |

### ランタイム依存

- [ripgrep (rg)](https://github.com/BurntSushi/ripgrep) — grep ソース、ファイル一覧のフォールバック
- [fd](https://github.com/sharkdp/fd) — ファイル一覧のソース

## アーキテクチャ

Bubbletea の Elm Architecture に従い、親 Model が `Pane` インターフェースを通じて Finder / Grep の2モードを統一的に扱う。

- `paneMsg` インターフェースで Msg の宛先ペインを自動ルーティング
- プレビュー読み込みは `tea.Cmd` による非同期 I/O
- 各ペインの `Update()` 戻り値を正しく反映する Elm Architecture 準拠の設計

詳細は [docs/architecture.md](docs/architecture.md) を参照。

## テスト戦略

TDD ベースで開発し、以下の形式を組み合わせる。

| 対象 | 手法 | パッケージ |
|------|------|-----------|
| ファジーマッチ、rg パーサー | Table-Driven + Fuzz | finder, grep |
| Model 状態遷移 | Msg → Model 検証 | ui |
| View() 出力 | Golden Test (teatest) | ui |
| リファクタリング安全網 | Scenario Test (Msg 駆動) | ui |
| panic 検出 | Fuzz Test (ランダム Msg 列) | ui |
| fd/rg 実行を伴うテスト | Integration Test | finder, grep |

```bash
go test ./...                                              # 全ユニットテスト
go test -tags=integration ./...                            # integration 含む
go test ./internal/ui -fuzz FuzzModelUpdateView -fuzztime 10s  # UI fuzz
go test ./internal/ui -update                              # ゴールデンファイル更新
```

## Development

[Claude Code](https://claude.ai/code) (Claude Opus) による Agent Coding で開発。
