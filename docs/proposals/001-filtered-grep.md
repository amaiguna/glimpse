# Proposal #001: Filtered Grep モードの追加

**Status:** In Progress (Phase 1 完了 2026-05-01)

## 概要

既存 Grep モードに「対象ファイル絞り込み」用の第2入力欄を追加し、ファイル名パターンと grep パターンを同時指定できるようにする。VSCode の検索ペイン (Search box + "files to include") に近い体験。

## 背景 / 動機

現状、Find と Grep はモード切替で排他利用となっている。実用上は「特定のファイル群に対してだけ grep をかけたい」シーン (例: `internal/ui` 配下の `*.go` だけで `func` を探す) が頻繁に発生し、その都度 Find で雰囲気を掴んでから Grep に切り替え直す手間が発生している。

## ゴール / 非ゴール

### ゴール

- Grep モード内でファイル絞り込みを併用できる
- include パターン未入力時は従来 Grep の挙動を完全に保つ (後方互換)
- 既存の Find / Grep モードの単独利用は引き続き可能

### 非ゴール

- Find モード単独の置き換え (Find は別ロールとして温存)
- 検索結果のランキング / 重要度ソート (現状の rg 順序を維持)
- ファイル絞り込みでの fuzzy 一致 (rg ネイティブ glob のみ)

## 設計判断

### D-1. モード構成: Grep モードを拡張する

**採用**: A. 既存 Grep モードに第2入力欄を追加する。

**理由**:
- include パターンが空のときは従来 Grep と完全等価 = 後方互換が自然
- VSCode の "files to include" もこの形で慣れ親しまれている
- モード数を増やさないので Tab 循環の認知負荷を上げない

**不採用**:
- B. 第3モード (Filtered Grep) として追加 — モード数増、選択肢が増える
- C. Grep 完全置き換え — 不要な大手術

### D-2. ファイル絞り込みの match 方式: glob

**採用**: rg ネイティブの `--glob` (繰返し可)。

**理由**:
- rg の機能を素通しにできる: `*.go`, `internal/**/*.go`, `!testdata/**` (negative glob) など標準的な書式
- ag / rg / git ls-files など類似 CLI とも一貫
- fuzzy 方式と違い、対象ファイルを事前に列挙する必要がなく速い
- Find 側 fuzzy との非対称性は「Find = ファイル選択 / Filtered Grep = grep 対象限定」とロールが異なるため許容

**不採用**:
- (b) fuzzy: `rg --files` で全列挙 → fuzzy filter → ヒットファイル群に対して再度 rg。手数が多い。
- (c) プレフィックスで切替: 学習コスト高。

#### 複数パターンの分離

1 行入力欄で複数 glob を渡したいケース (例: `*.go *.md`)。**仮決定**: 空白区切りで split し、それぞれを `--glob` として rg に渡す。空白を含むパターン (実用上ほぼ無い) は対象外。要再考点として Phase 3 でユーザフィードバックを受けて確定する。

### D-3. キーバインド: Tab 循環は維持、Shift+Tab で入力欄間移動

**採用**:
- `Tab` — 引き続き Find ↔ Grep のグローバルモード切替
- `Shift+Tab` — Grep モード内で grep 入力欄 ↔ include 入力欄のフォーカス移動

**理由**:
- 既存の Tab 循環の挙動を変えない (記憶の上書きを発生させない)
- 多くの TUI / フォーム UI で Shift+Tab は逆方向 / セカンダリの自然な選択

**未決**: `Ctrl+I` を別名として併設するかは Phase 4 で UX を見て判断。

### D-4. レイアウト: 2 行常時表示、include 空ならグレーアウト

**採用**: include 入力欄を grep モード時に常に表示。空のときは placeholder + grayed out にして「機能の存在を発見しやすく」する。

```
 [Grep] > <grep pattern>
 files:  > <glob pattern>          ← 空時は grayed out + placeholder
┌─ list ──────────┬─ preview ─────────────────┐
│ a.go:10:hit     │ ...                       │
│ b.go:5:hit      │                           │
└─────────────────┴───────────────────────────┘
```

**理由**: include 空時に行を畳むと存在が discover されにくく、機能が死ぬ。常時表示で発見可能性を担保し、グレーアウトで視覚ノイズは抑える。

**副作用**: ヘッダー高さが 1 行 → 2 行に増えるため、`contentHeight()` の計算と既存ゴールデンテストが影響を受ける。

### D-5. Pane インターフェース見直しを前提とする (#006 解消が前提)

現状 Pane は 12 メソッド (#010 で `SetErr` 追加済)。複数 textinput を持つ Grep モードを既存 Pane に押し込むと:

- `TextInputView() string` 単一返却の前提が崩れる
- include 用の query / filter 解析メソッドが追加で必要 → さらに肥大化
- Finder 側がパススルー実装するメソッドが増える (ISP 違反が深まる)

→ #006 (Pane インターフェース肥大化) の再評価ラインを完全に超えており、本機能と合わせて Pane を分割するのが筋。

#### 分割の方向性 (詳細は #006 で決める)

仮の見立て。最終形は #006 解消時に確定する:

```go
// 親 Model が必ず使う基本契約
type Pane interface {
    Update(tea.Msg) (Pane, tea.Cmd)
    View() string
    Query() string
    IsLoading() bool
    Err() error
    SetErr(error)
}

// ヘッダー描画用 (複数入力を許容)
type HeaderRenderer interface {
    HeaderViews() []string  // 各入力欄の View を返す (1 個 or 複数)
}

// 選択 / オープン
type Selector interface {
    SelectedItem() string
    FilePath() string
    OpenTarget() (file string, line int)
}

// プレビュー装飾
type PreviewDecorator interface {
    PreviewRange(visibleHeight int) int
    DecoratePreview(content string, width int) string
}
```

親 Model は型アサーションで必要なロールを取得する。Filtered Grep は `HeaderRenderer.HeaderViews()` で 2 行返す実装になる。

## 段階的ロードマップ

### Phase 1: #006 解消 — Pane インターフェース再設計 ✅ 完了 (2026-05-01)

**前提**: 本提案。スコープ:

- Pane を上記方向性で分割 → 完了 (Pane / HeaderRenderer / Selector / PreviewDecorator)
- 既存 Finder / Grep の挙動を維持しつつ親 Model 側を型アサーション化 → 完了
- ゴールデンテスト・シナリオテストでの非後退を担保 → 完了 (golden 差分ゼロ、fuzz panic なし)

**成功基準**: 既存テストすべてパス、`Pane` 本体メソッド数が 6〜8 程度に収まる。
**結果**: Pane 本体は 6 メソッド (Update / View / Query / IsLoading / Err / SetErr)。詳細は [#006](../issues/006-pane-interface-bloat.md)。

### Phase 2: include 入力欄 UI 追加

- `GrepModel` に `includeInput textinput.Model` を追加
- `HeaderRenderer.HeaderViews()` を 2 要素返す
- `Shift+Tab` での 2 入力欄間 focus 移動を実装
- レイアウト: 2 行ヘッダー対応 + grayed-out placeholder
- include パターンは UI 状態として保持するだけで、まだ rg には渡さない (Phase 3 で接続)

**成功基準**: include 欄に文字が打てる / Shift+Tab で行き来できる / 既存 Grep 検索は変化しない。

### Phase 3: rg --glob 配線

- include の入力値を空白で split し、各トークンを `--glob` 引数として rg に渡す
- 空白区切り仕様は doc に明記
- 既存 debounce タイミングに include の変更も乗せる (どちらの入力欄でも 300ms デバウンス → 検索発火)

**成功基準**:
- include 欄に `*.go` を入れて grep すると `.go` ファイルだけがヒット
- include 欄に `!testdata/**` を入れると `testdata` 配下が除外される

### Phase 4: ポリッシュ

- ヘルプ表示 (`?` キー検討) でのキーバインド説明
- include の不正な glob → エラー表示は #007 と同じステータス行ルートに乗せる (rg が stderr を返してくれる)
- 必要に応じて `Ctrl+I` を Shift+Tab の別名として追加

## 未確定事項 / 要再考点

| 項目 | デフォルト | 要件確定タイミング |
|---|---|---|
| 複数 glob の分離方法 | 空白区切り | Phase 3 でユーザフィードバック後 |
| include 欄の placeholder 文言 | `e.g. *.go !testdata/**` | Phase 2 |
| `Ctrl+I` 併設 | 後回し | Phase 4 |
| Grep モード切替 (`Tab`) で include 欄もリセットするか | リセット (既存 Reset と同調) | Phase 2 |
| `Reset` 時の include 欄保持 (モード切替を跨いで残すか) | 残さない | Phase 2 |

## 影響範囲の見積り

| 領域 | 内容 |
|---|---|
| `internal/ui/pane.go` | インターフェース分割 (Phase 1) |
| `internal/ui/finder.go` | 新インターフェースに合わせて受動的な変更 |
| `internal/ui/grep_model.go` | textinput 2 個化 + glob 引数組み立て |
| `internal/ui/model.go` | 型アサーションでヘッダー / セレクタ取得 |
| `internal/grep/grep.go` | `Search(ctx, pattern, globs []string)` 等にシグネチャ拡張 |
| ゴールデン全般 | ヘッダー 2 行化 / include grayed-out 対応で大量更新 |
| シナリオテスト | include 入力 + 検索のシナリオ追加 |

## 関連

- 前提 issue: [#006 Pane インターフェースの肥大化](../issues/006-pane-interface-bloat.md)
- 表示・エラー経路で再利用: [#007](../issues/007-grep-broken-regex-ui-collapse.md), [#009](../issues/009-pane-error-collapses-view.md)
- アーキテクチャ概観: [`docs/architecture.md`](../architecture.md)
