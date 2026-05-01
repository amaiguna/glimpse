# Proposals

glimpse-tui に対する大規模な機能追加・設計変更の提案 (RFC) を保管する場所。バグ・負債は `docs/issues/`、セキュリティ監査は `docs/security_audit/` を使い、それ以外の前向きな設計議論はここに置く。

## 運用ルール

- **番号は再利用しない**: クローズ後もファイルは削除せず `Status:` を更新する。過去の PR / コミットメッセージから `proposal #NNN` を辿れる状態を維持する。
- **Status の値**:
  - `Draft` — 議論中 / 着地前
  - `Approved` — 方針合意済 / 実装待ち
  - `In Progress` — 実装フェーズに入った
  - `Implemented (YYYY-MM-DD)` — 実装完了
  - `Rejected (YYYY-MM-DD)` — 採用しない決定 (理由を併記)
  - `Withdrawn (YYYY-MM-DD)` — 提案者が撤回
  - `Superseded by #NNN` — 後続 proposal に置き換え
- **Approved 以降の編集**: 軽微な誤記訂正、Status 更新、フェーズ進捗記録のみ許容。設計判断本体は当時の意思決定の記録として残す。再設計が必要なら新番号で立て直し、旧 proposal を `Superseded` する。
- **次の番号**: `ls docs/proposals/` の最大番号 + 1。

## Index

| # | Title | Status | 概要 |
|---|---|---|---|
| 001 | [Filtered Grep モードの追加](001-filtered-grep.md) | In Progress | 既存 Grep モードにファイル絞り込み (glob) 入力欄を追加。Phase 1-3 完了 2026-05-01 (Pane 分割 / UI / rg --glob + 自動 substring wrap)。残 Phase 4 (ポリッシュ)。 |

## 関連

- バグ / 負債: [`docs/issues/README.md`](../issues/README.md)
- セキュリティ監査: [`docs/security_audit/README.md`](../security_audit/README.md)
- プロジェクト指針: [`CLAUDE.md`](../../CLAUDE.md)
