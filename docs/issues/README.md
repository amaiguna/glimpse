# Issues

glimpse-tui の課題管理。`go-cli-security-audit` skill による監査とは別に、UX バグ・設計上の負債・リファクタ候補等を番号付きで追跡する。

## 運用ルール

- **番号は再利用しない**: クローズ後もファイルは削除せず、冒頭の `Status:` を `Closed (YYYY-MM-DD)` に更新する。番号の永続的な予約により、過去の PR / コミットメッセージから `#NNN` を辿れる状態を維持する。
- **Status の値**:
  - `Open` — 未着手 / 進行中
  - `Closed (YYYY-MM-DD)` — 解消済（解消サマリを 1〜2 行で併記）
  - `Won't fix (YYYY-MM-DD)` — 対応しない決定（理由を併記）
  - `Superseded by #NNN` — 後続 issue に置き換えられた
- **Closed 後の編集**: `Status:` 行の追記、リンク修正、誤記訂正のみ許容。本文の対応方針・判断パートは当時の意思決定の記録として残す。再評価が必要なら新番号で立て直し、旧 issue は `Superseded` する。
- **次の番号**: `ls docs/issues/` の最大番号 + 1。Closed 済を含めた max を基準にすることで衝突しない。

## 適用範囲

このルールは **#006 以降に適用**。#001〜#005 はこのルール導入前に削除済のため履歴を持たない。

## Index

| # | Title | Status | 解消サマリ |
|---|---|---|---|
| 006 | [Pane インターフェースの肥大化](006-pane-interface-bloat.md) | Closed (2026-05-01) | Pane を 4 ロール (Pane / HeaderRenderer / Selector / PreviewDecorator) に分割。本体 6 メソッド。Model は type assertion でオプショナルロール取得。 |
| 007 | [grep モードで不正 regex 入力時に UI が崩壊する](007-grep-broken-regex-ui-collapse.md) | Closed (2026-04-26) | `simplifyGrepError` で stderr 部分のみ surface。前回ヒットを維持。#008 + #009 が前提。 |
| 008 | [外部コマンドの stderr 喪失でエラー診断が無効化](008-external-command-stderr-lost.md) | Closed (2026-04-26) | grep / finder 両方に `CmdError{ExitCode, Stderr, Err}` 導入。`runWithLimit` が stderr を 64KB 上限で取り込む。 |
| 009 | [pane.Err() が真の時に View を全崩壊させる UX](009-pane-error-collapses-view.md) | Closed (2026-04-26) | View 早期 return 廃止。エラーは header 直下のステータス行として通常レイアウト維持。 |
| 010 | [エディタ起動失敗が黙殺される](010-editor-launch-failure-silent.md) | Closed (2026-04-26) | `Pane.SetErr` 追加 + `exec.LookPath` 事前検証 + `EditorFinishedMsg` 反映で #009 のステータス行に表示。 |

## 関連

- 機能提案 (RFC): [`docs/proposals/README.md`](../proposals/README.md)
- セキュリティ監査: [`docs/security_audit/README.md`](../security_audit/README.md)
- プロジェクト指針: [`CLAUDE.md`](../../CLAUDE.md)
