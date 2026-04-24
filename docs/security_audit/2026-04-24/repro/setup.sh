#!/usr/bin/env bash
# glimpse-tui Security Audit — High 項目再現用セットアップ
#
# ペイロードは観察可能だが非破壊のものだけを使用:
#   - OSC 0 でタイトル書き換え（ターミナル再起動で戻る）
#   - ANSI SGR で色変更（\x1b[0m で戻す）
#   - \x1b[2J\x1b[H で画面クリア（Ctrl+L 相当の一時的影響）
#
# OSC 52 (クリップボード書き込み) や OSC 8 (リンク) などの副作用が大きいものは使わない。
#
# このスクリプトは冪等で、再実行で各ファイルが上書き再生成される。

set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# ---- H-1 / H-3: ファイル内容にエスケープシーケンスを仕込む ----
# SEARCHME は Grep モードでヒットさせるマーカー。
{
  printf 'Line 1 normal text SEARCHME\n'
  printf 'Line 2 title hijack: \x1b]0;HIJACKED_BY_PREVIEW\x07continuation SEARCHME\n'
  printf 'Line 3 color injection: \x1b[41;97mWHITE_ON_RED\x1b[0m SEARCHME\n'
  printf 'Line 4 clear screen: \x1b[2J\x1b[HSCREEN_CLEARED SEARCHME\n'
  printf 'Line 5 normal text SEARCHME\n'
} > payload-content.txt

# ---- H-2: ファイル名自体にエスケープシーケンス ----
# このファイル名は制御文字を含むため git に commit しない運用。
# 既存のものがあれば削除してから再作成する。
find . -maxdepth 1 -type f -name $'name_*HIJACKED*' -delete 2>/dev/null || true
touch $'name_\x1b[41;97mHIJACKED_FILENAME\x1b[0m_.txt'

# ---- 通常のファイル（比較対照） ----
printf 'This is a completely normal file.\nSEARCHME marker here too.\n' > normal.txt

echo "Setup complete. Files in $DIR:"
ls -la
