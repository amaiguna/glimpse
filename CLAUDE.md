# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

telescope-tui: A standalone TUI fuzzy finder inspired by Neovim's Telescope plugin. Runs independently from any editor in the terminal.

## Tech Stack

- **Language:** Go
- **TUI framework:** charmbracelet/bubbletea (Elm Architecture)
- **Layout/styling:** charmbracelet/lipgloss
- **UI components:** charmbracelet/bubbles (textinput)
- **Fuzzy matching:** sahilm/fuzzy
- **Syntax highlighting:** alecthomas/chroma
- **Runtime deps:** ripgrep (`rg`), fd

## Build & Run

```bash
go build -o telescope-tui .
go run .
```

## Testing

```bash
# Unit tests only
go test ./...

# Unit + integration tests (requires fd, rg)
go test -tags=integration ./...

# Run a single test
go test ./internal/grep -run TestParsRgJSON

# Run fuzz tests (grep parser)
go test ./internal/grep -fuzz FuzzParseRgJSON

# Run fuzz tests (UI panic 検出)
go test ./internal/ui -fuzz FuzzModelUpdateView -fuzztime 10s

# Update golden files
go test ./internal/ui -update
```

Test files live alongside source code (`_test.go`). Integration tests use `//go:build integration` build tag.

Uses `testify/assert` for assertions and `charmbracelet/x/exp/teatest` for golden tests.

## Coding Conventions

- godoc コメントは日本語で書く。公開型・公開関数には必ず付ける。

## Architecture

Pane インターフェースによるポリモーフィズムで2つのモードを統一的に扱う:

- **File Finder (`FinderModel`)** — `fd` or `rg --files` でファイル一覧取得、ファジーフィルタリング、`$EDITOR` で開く
- **Live Grep (`GrepModel`)** — `rg --json` をデバウンス付きで実行、ファイル名一覧を表示、`$EDITOR +line file` で開く
- **Preview pane** — 右ペインにファイルプレビュー（シンタックスハイライト付き）。Grep モードではヒット行を中央配置し、マッチ単語をハイライト

### Msg ルーティング

- `paneMsg` インターフェース（`PaneTarget() Mode`）を実装した Msg は、`Update()` で自動的に対応ペインにルーティングされる
- ペイン固有の Msg を追加する場合は `PaneTarget()` を実装するだけで `Update()` の変更不要

### プレビューの非同期読み込み

- `previewCmd()` が `tea.Cmd` を返し、`PreviewLoadedMsg` として非同期に結果を受け取る
- ファイルパスと開始行の照合で古いプレビューの上書きを防止

## Test Strategy

- **Table-driven tests** for logic (fuzzy matching, rg JSON parsing)
- **Fuzz tests** (Go 1.18+ `testing.F`)
  - `internal/grep`: rg パーサーの不正入力耐性
  - `internal/ui`: ランダム Msg 列での panic 検出（キー入力・マウス・リサイズ・モード切替の混合）
- **Golden tests** (`teatest`) comparing `View()` output against golden files
- **Model tests** verifying Msg → Model state transitions and Cmd side effects
- **Scenario tests** — Msg 駆動で内部フィールドに触れず、Update → View の流れだけで動作検証するリファクタリング安全網
