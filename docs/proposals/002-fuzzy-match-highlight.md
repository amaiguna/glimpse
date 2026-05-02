# Proposal #002: Finder / Grep ファジーマッチのハイライト表示

**Status:** Approved (2026-05-02)

## 概要

Finder ペイン (Files モード) と Grep モード左ペインの include 入力時に、ファジーマッチした文字位置を ANSI ハイライトで可視化する。
現状はマッチしたアイテムが何故ヒットしたのか視覚的にわからず、fuzzy 入力の効果がユーザーに伝わらない。

## 背景 / 動機

- Finder ペインは fuzzy マッチした文字列の **どの文字** がマッチしたか見せていない (selected 行は accent でハイライトされるが、これは「選択中」の意味で「マッチ」の意味ではない)
- `finder.FuzzyFilter` は既に `MatchedIndexes []int` (マッチした各文字のバイト位置) を返している。描画側で捨てているだけ
- proposal #001 で Grep 左ペインにも同じ fuzzy が入ったので、同じ問題が広がった
- Grep プレビューでは既に同等のハイライト機構 (`highlightMatches` + `matchHlStart`/`End`) があり、ユーザーは「マッチ箇所のハイライト」UX に親しんでいる

## ゴール / 非ゴール

### ゴール

- Finder ペイン: クエリが非空のとき、各表示アイテムの MatchedIndexes 位置をハイライト
- Grep 左ペイン: include が非空のとき、各 `"file:line:text"` の **ファイルパス部分のみ** ハイライト
- ハイライト style は当面 grep プレビューと同じ ANSI を使うが、**後から付け替え容易な構造** にする

### 非ゴール

- ハイライト color theme カスタマイズ機能の追加 (将来課題)
- grep プレビュー側ハイライト実装の改修 (別 proposal)
- sanitize 必須なファイル (ESC 含むパス) でのハイライト整合保証 — corner case として best-effort

## 設計判断

### D-1. ハイライト style: grep プレビューと「偶然同じ」ANSI、定数は分離

**採用**: 新規に `fuzzyMatchHlStart` / `fuzzyMatchHlEnd` 定数を `styles.go` に追加する。値は当面 `matchHlStart` / `matchHlEnd` (青背景 + bold) と同じ。

**理由**:
- 同じ見た目で「マッチ箇所」の意味的一貫性を保てる
- ただし「同じ ANSI を使っている」のは現時点での偶然であって、設計上の依存ではない。将来 Finder と grep プレビューでハイライト style を分けたくなったとき、定数値を変えるだけで済むようにする
- ユーザの明示要望: 「あくまでハイライト仕様に grep と偶然同じ設定を使っている、というだけで、後から付け替え可能性を用意にしておいてほしい」

**不採用**: `matchHlStart` を直接流用 — grep preview との結合が強くなり、後から分離コストが上がる

### D-2. ハイライト適用範囲

**採用**:
- **Finder ペイン**: クエリ非空時、表示アイテム全体の MatchedIndexes 位置をハイライト
- **Grep 左ペイン**: include 非空時、各アイテムの **ファイルパス部分** (`parseGrepItem` で抽出される `file` 部分) のみハイライト。`:line:text` 側はハイライトしない

**理由**:
- include はファイルパスへの fuzzy match であり、行内容 (text) には関係ない。混在させない
- Grep プレビューは別の grep pattern を反映する別経路のハイライトがあり、左ペインの責務とは分離

### D-3. 空クエリは無効化

**採用**: クエリ/include が空のときはハイライトを完全に skip。

**理由**:
- `FuzzyFilter("")` は内部仕様で「全文字インデックス」を返すが、これをそのまま render に流すと**全文字が背景色になり**「マッチなし」と区別がつかなくなる
- 性能上も空クエリ時は描画コストを増やしたくない
- 実装は **render 側で「query 非空のとき以外はハイライトを skip」** する単純条件で対応 (FuzzyFilter の内部仕様には依存しない)

### D-4. truncate との相性: best-effort

**採用**: `ansi.Truncate` (既に finder/grep の View で使用中) をそのまま使用。ハイライト ANSI 挿入後に truncate するので、表示に収まる範囲だけハイライトが見える。

**詳細**:
- `ansi.Truncate` は ANSI シーケンスを認識して可視幅を計算するため、ハイライト ANSI を挟んでも壊れない
- 表示幅を超えてハイライト範囲がはみ出した場合 → 見える部分だけハイライト適用 (中途半端な ANSI がそのまま見えることはない)
- 専用のフォールバック処理は書かない (既存ライブラリに任せる)

### D-5. sanitize 相互作用: 後回し / セキュリティ優先

**採用 (暫定)**:
- 現状の `sanitize.ForTerminal` 適用順序を維持 (描画前に sanitize)
- sanitize 適用後にハイライト ANSI を注入する設計とし、ESC バイトを含むパスでは MatchedIndexes が sanitize 後の文字列と ずれる corner case が発生する
- 表示そのものはセキュアに維持 (ANSI 注入攻撃を防ぐ) が最優先

**理由**:
- 通常のファイルパスでは sanitize は no-op に近く、indexes はそのまま使える
- ESC 含むパス名は実用上極めて稀。整合保証は低優先

**今後の課題**: ESC 含むパス名で MatchedIndexes を sanitize 後の文字列に再マップする方法は別途検討 (Phase 3)

### D-6. データ構造

**採用**:

`FinderModel.items []string` を構造体スライスに変更:

```go
type fuzzyItem struct {
    Str            string
    MatchedIndexes []int  // クエリ空時 / 不要時は nil
}
```

`SelectedItem()` / `FilePath()` / `OpenTarget()` は `Str` を返す形に内部修正 (Pane / Selector 契約は不変)。

`GrepModel` 側は items の構造を `file:line:text` 形式から変えたくないので、別経路で MatchedIndexes を保持:

```go
type GrepModel struct {
    items                 []string         // 既存通り "file:line:text"
    pathMatchedIndexes    map[string][]int // ファイルパス → MatchedIndexes (include 非空時のみ)
    ...
}
```

または各 item と並走するスライスで保持 (実装時に決定)。

## 段階的ロードマップ

### Phase 1: スタイル定数 + ヘルパ追加

- `styles.go` に `fuzzyMatchHlStart` / `fuzzyMatchHlEnd` 定数を追加 (値は当面 `matchHlStart`/`End` と同じ)
- `highlightAtIndexes(s string, indexes []int) string` ヘルパ追加 (rune-aware に各文字位置にハイライト ANSI を挿入)
- 単独の table-driven テストで pin

**成功基準**: `highlightAtIndexes("abc", []int{1})` が "a" + ハイライト + "b" + リセット + "c" を返す

### Phase 2: Finder ペインのハイライト

- `FinderModel.items` を `[]fuzzyItem` に変更
- `applyFilter()` で MatchedIndexes も保持
- `View()` でクエリ非空時にハイライト適用 (sanitize → highlight → truncate の順)
- `SelectedItem` / `FilePath` / `OpenTarget` の内部実装を `.Str` 経由に
- ゴールデン更新

**成功基準**: クエリ "intui" で `internal/ui/model.go` の i, n, t, u, i 各文字位置に ANSI ハイライトが乗る (TestMain 強制 ANSI 環境で assert)

### Phase 3: Grep 左ペインのハイライト

- `GrepModel` に `pathMatchedIndexes` 等を追加 (実装は方式自由)
- `handleDebounceTick` (または fuzzyFilterFiles 周辺) で MatchedIndexes を保持
- `View()` で include 非空時にファイルパス部分のみハイライト
- ゴールデン更新

**成功基準**: include "CLAUDE" で grep 結果リストの `CLAUDE.md` 部分にハイライトが乗り、`:42:matched text` 部分は素通し

### Phase 4: ポリッシュ (optional / 将来)

- sanitize 必須パスでの best-effort MatchedIndexes 再マップ
- ハイライト style カスタマイズ (theming) — 必要が出てから

## 影響範囲の見積り

| 領域 | 内容 |
|---|---|
| `internal/ui/styles.go` | `fuzzyMatchHlStart`/`End` 定数 + `highlightAtIndexes` ヘルパ |
| `internal/ui/finder.go` | `items []fuzzyItem` に変更、View にハイライト統合、Selector 系の内部修正 |
| `internal/ui/grep_model.go` | path MatchedIndexes 保持機構、View にファイルパス部分ハイライト統合 |
| ゴールデン | finder + grep 系で再生成 |
| テスト | `highlightAtIndexes` の table-driven、Finder/Grep の View ハイライト pin |

## 関連

- 前提: [Proposal #001 Filtered Grep](001-filtered-grep.md) で Grep にも fuzzy が入ったので Phase 3 が成立
- 関連 helper: `highlightMatches()` (grep preview, substring match) — 別関数として共存。共通化は不要
- 既存ハイライト ANSI: `matchHlStart` / `matchHlEnd` (`styles.go`) — D-1 で定数を切り離して独立進化を可能に
