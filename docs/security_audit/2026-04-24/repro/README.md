# High 項目 再現手順

監査レポート本体: [`../report.md`](../report.md)

## ディレクトリ構成

| ファイル | 用途 | git |
|---|---|---|
| `README.md` | この手順書 | 追跡 |
| `setup.sh` | 再現ファイル生成（冪等） | 追跡 |
| `payload-content.txt` | 内容に ESC を含むファイル（H-1, H-3） | 追跡 |
| `normal.txt` | 比較対照 | 追跡外（setup.sh が生成） |
| `name_<ESC>[41;97mHIJACKED<ESC>[0m_.txt` | 名前に ESC を含むファイル（H-2） | **追跡外**（setup.sh が生成） |

ファイル名に制御文字を含むものは git 管理下に置かない。GUI ファイラーや一部のエディタ／補完で表示が壊れるため、`setup.sh` で都度生成する運用にしている。

## 前提

```bash
# プロジェクトルートで
go build -o glimpse .
# 再現ディレクトリで生成 & 起動
cd docs/security_audit/2026-04-24/repro
bash setup.sh
../../../../glimpse    # または絶対パスで glimpse バイナリを叩く
```

ペイロードは**観察可能だが非破壊**のものだけ（タイトル書換・色変更・画面クリア）。副作用の強い OSC 52（クリップボード）や OSC 8 リンクは意図的に避けている。

## H-1: ファイル内容経由のエスケープ注入（Preview）

1. glimpse を Finder モード（起動直後）で起動。
2. 矢印キーで `payload-content.txt` を選択。右ペインに preview が出る。
3. あるべき挙動（安全なアプリ）: 行の中の `\x1b]0;HIJACKED_BY_PREVIEW\x07` が可視化（例 `\x1b]0;HIJACKED...`）され、タイトルバーや画面状態は変わらない。
4. 現状（脆弱）:
   - ターミナルのタイトルバーが `HIJACKED_BY_PREVIEW` に書き換わる。
   - Line 3 の `WHITE_ON_RED` が赤背景+白文字で描画される（chroma のハイライトとは別の、ファイル内容に由来する色）。
   - Line 4 の `\x1b[2J\x1b[H` がそのまま流れ、preview の描画タイミングによっては画面が一度クリアされる。

タイトルバーを元に戻す: `printf '\x1b]0;%s\x07' "$USER@$(hostname)"`、または新しいタブを開く。

## H-2: ファイル名経由のエスケープ注入（ファイルリスト描画）

1. 同じ起動で、左ペインのファイルリストに `name_<ESC>[41;97mHIJACKED_FILENAME<ESC>[0m_.txt` が並ぶ。
2. あるべき挙動: 制御文字が `\x1b` のように可視化される。
3. 現状: `HIJACKED_FILENAME` が **赤背景+白文字**で描画される。`fd` / `rg --files` が出したファイル名を `View()` がそのまま流しているため。
4. 応用: ファイル名に `\x1b]0;TITLE\x07` を入れればタイトル書換、`\x1b[2J\x1b[H` を入れれば画面クリアも可能。

確認用（シェル側）:

```bash
ls | cat     # cat 経由で生バイトを見る
ls -b        # エスケープを可視化した表示
xxd name_*HIJACKED* | head
```

## H-3: grep 行内容経由のエスケープ注入（Grep モード）

1. glimpse 起動後、`Tab` で Grep モードに切替。
2. クエリに `SEARCHME` と入力。
3. 左ペインに `payload-content.txt` の各行が `file:line:text` 形式で並ぶ。
4. Line 2 の行を選ぶと preview で H-1 と同じ挙動。
5. それとは別に、**左ペインの grep 行自体**にも同じ制御シーケンスが含まれているため、描画時にタイトル書換や色変化が起こる（`parseGrepItem` が抽出した text 部分は truncate はするが制御文字は残る）。

## クリーンアップ

生成された追跡外ファイルを消したい場合:

```bash
find . -maxdepth 1 -type f -name 'name_*HIJACKED*' -delete
rm -f normal.txt
```

`payload-content.txt` は追跡対象なので、消したら `git restore` で戻せる。
