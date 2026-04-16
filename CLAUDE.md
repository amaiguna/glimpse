# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

telescope-tui: A standalone TUI fuzzy finder inspired by Neovim's Telescope plugin. Runs independently from any editor in the terminal.

## Tech Stack

- **Language:** Go
- **TUI framework:** charmbracelet/bubbletea (Elm Architecture)
- **Layout/styling:** charmbracelet/lipgloss
- **UI components:** charmbracelet/bubbles
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

# Run fuzz tests
go test ./internal/grep -fuzz FuzzParseRgJSON

# Update golden files
go test ./internal/ui -update
```

Test files live alongside source code (`_test.go`). Integration tests use `//go:build integration` build tag.

Uses `testify/assert` for assertions and `charmbracelet/x/exp/teatest` for golden tests.

## Coding Conventions

- godoc コメントは日本語で書く。公開型・公開関数には必ず付ける。

## Architecture

Two hardcoded modes (no source/picker abstraction yet):

- **File Finder** — sources file list from `fd` or `rg --files`, applies fuzzy filtering, opens selection in `$EDITOR`
- **Live Grep** — runs `rg --json` with debounce, displays file + line + match, opens `$EDITOR +line file`
- **Preview pane** — right-side file preview with syntax highlighting

Bubbletea's Elm Architecture: `Model` receives `Msg`, returns updated `Model` + `Cmd` (side effects).

## Test Strategy

- **Table-driven tests** for logic (fuzzy matching, rg JSON parsing)
- **Fuzz tests** (Go 1.18+ `testing.F`) for fuzzy match and rg parser robustness
- **Golden tests** (`teatest`) comparing `View()` output against golden files
- **Model tests** verifying Msg → Model state transitions and Cmd side effects
