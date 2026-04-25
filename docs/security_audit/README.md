# セキュリティ監査ログ

glimpse-tui に対するセキュリティ監査の履歴。`go-cli-security-audit` skill のワークフロー（Phase 1〜4）に基づき定期実施する。

## 運用ルール

- **日付ディレクトリは immutable**: クローズ後は基本いじらない。誤記訂正やリンク修正のみ許容。再評価・再発見はその時点の新監査として別ディレクトリに記録する。
- **Status の 3 値**:
  - `Open` — 進行中
  - `Closed` — 全項目が対応済または明示的に Won't-fix で受け入れ済
  - `Superseded` — 後続監査に置き換えられた（通常は使わない）
- **持ち越し項目**: Won't-fix や次回送りの項目は、次回監査時に当該レポートへリンクで参照する（コピペしない）。
- **クロージング条件**: Phase 1〜4 の全項目について「対応済」または「Won't-fix（理由明記）」のいずれかになり、`go test` / `go vet` / `staticcheck` / `govulncheck` が無指摘であること。

## 監査履歴

| 日付 | Status | 対象コミット | 主な発見 / 対応 | レポート |
|---|---|---|---|---|
| 2026-04-24 | Closed (2026-04-25) | `cf8e186` | High×3 (TUI エスケープ注入: preview / ファイル名 / grep 行) を `internal/sanitize` 新設で解消。Medium×3 (preview OOM / エディタ引数注入 / `exec.CommandContext` 不使用)、Low の L-2 (PATH) / L-3 (env)、Info×4 も対応済。L-1 (symlink) は Won't-fix。Phase 4 Fuzz 3 種追加（うち `FuzzParseGrepItem` で line==0 曖昧バグ 1 件を fuzz 駆動で発見・修正）。 | [report.md](2026-04-24/report.md) |

## 次回監査の手順

1. `docs/security_audit/<YYYY-MM-DD>/` を作成
2. `go-cli-security-audit` skill を起動して Phase 1〜4 を実施
3. レポート冒頭に `Status: Open` を記載し、対象コミット・Go バージョン・対象範囲を明記
4. 持ち越し / 再評価対象（Won't-fix を含む）は前回レポートにリンクで参照
5. 全項目に決着が付いたら `Status: Closed (YYYY-MM-DD)` + クロージングノートを追加し、本 index にエントリを追加

## 関連リンク

- skill: `~/.claude/plugins/.../go-cli-security-audit/`（`go-cli-security-audit` で起動）
- プロジェクト指針: [`CLAUDE.md`](../../CLAUDE.md)
