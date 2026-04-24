# 脅威モデリング用プロンプトテンプレート

Phase 2 で使用する。プロジェクトのattack surface を網羅的に洗い出すためのプロンプト。Claude自身(または別セッションのClaude Code)に対して投げる想定。

## 使い方

1. プロジェクトのルートディレクトリをClaude(Claude Code等)のcontext に取り込ませる
2. 以下のプロンプトを投げる
3. 出力されたデータフロー表を Phase 3 の入力として使う

## プロンプト本体

```
このGoプロジェクトのセキュリティ脅威モデリングを行ってください。以下の手順で進めてください。

## Step 1: 信頼境界を越える流入点を全て列挙

以下のカテゴリそれぞれについて、プロジェクト内の該当箇所(ファイル:行番号)を列挙してください:

1. コマンドライン引数の読み取り(`os.Args`, `flag`, `cobra`, `pflag` 等)
2. 標準入力からの読み取り(`os.Stdin`, `bufio.NewReader(os.Stdin)` 等)
3. 環境変数の読み取り(`os.Getenv`, `os.LookupEnv`, `os.Environ`)
4. 設定ファイルの読み取り(YAML, TOML, JSON, INI等)
5. 外部プロセスの出力受信(`exec.Command.Output`, `StdoutPipe`等)
6. ネットワーク経由の入力(HTTP応答、TCP/UDPソケット、gRPC等)
7. ファイルシステムからの読み取り(特にユーザー指定のパスから)
8. キーボード/マウス等の端末入力(Bubbleteaの `tea.Msg`、ncurses等)

各項目について、「どこで受け取り、どの型で保持され、どこに伝播するか」の概要も付けてください。

## Step 2: 危険なsinkを全て列挙

以下のカテゴリそれぞれについて、該当箇所を列挙してください:

1. 外部プロセスの実行(`exec.Command`, `exec.CommandContext`, `syscall.Exec` 等)
2. ファイル書き込み(`os.Create`, `os.OpenFile`, `os.WriteFile` 等)
3. ファイル読み取り先のパス決定箇所(パストラバーサル懸念)
4. ネットワーク送信(HTTP クライアント、TCP接続先決定)
5. ターミナルへの描画(`fmt.Print*`, Bubbletea の `View()` 返り値、Lipglossスタイル適用)
6. ログ出力(秘密情報混入の懸念)
7. デシリアライゼーション(`encoding/gob`, `json.Unmarshal` 等、特にポリモーフィックな型)
8. SQLクエリ実行(該当あれば)
9. 正規表現コンパイル(ユーザー入力からの正規表現構築によるReDoS懸念)

## Step 3: データフロー表の作成

Step 1 の流入点から Step 2 の sink に到達しうる経路を表にまとめてください:

| 流入点 | 経由する関数 | 到達するsink | リスク分類 | 既存の防御 |
|--------|--------------|--------------|------------|------------|
| 例: `os.Args[1]` (ファイルパス) | `loadConfig()` | `os.Open` | パストラバーサル | `filepath.Clean` 適用済み |

「リスク分類」は以下から選んでください:
- コマンドインジェクション
- パストラバーサル
- ターミナルエスケープ注入
- ReDoS / DoS
- 機密情報漏洩
- 権限昇格
- デシリアライゼーション攻撃
- SSRF
- その他(具体的に記述)

## Step 4: カバレッジ評価

Step 3 の各行について、以下を判定してください:

- [Covered by tool]: Phase 1 の静的解析ツール(govulncheck, gosec等)が検出可能な一般的パターン
- [Needs manual review]: ツールでは検出困難で、Phase 3 の手動レビューが必要
- [Low risk]: 理論的には経路があるが、実際の悪用可能性は低い(理由を明記)

## 重要な注意

- 推測ではなく、**実際にコードを読んだ結果**として報告してください。該当箇所をGrep/Readで確認した上で列挙してください。
- 網羅性を優先し、深掘りは Phase 3 に譲ってください。
- 「この関数は安全です」と判定する際は、判定根拠(どういう防御が入っているか)を必ず明記してください。
```

## 出力例(参考)

```markdown
## Step 1: 流入点

### コマンドライン引数
- `cmd/root.go:42`: `--query` フラグ (string) → main.runFuzzy() → searchEngine.Run()
- `cmd/root.go:48`: `--preview-dir` フラグ (string) → previewer.New()
- `cmd/root.go:55`: 位置引数 args[0] (検索ディレクトリ) → finder.Init()

### 環境変数
- `config/config.go:23`: `FUZZY_CONFIG_PATH` → configLoader.Load()
- `internal/chroma.go:15`: `NO_COLOR` → styler.Init()

### 外部プロセス出力
- `search/ripgrep.go:67`: `rg --json` の stdout → rgParser.Parse()
- `search/fd.go:34`: `fd --type f` の stdout → fileListReader.Read()

## Step 2: sink

### 外部プロセス実行
- `search/ripgrep.go:54`: `exec.CommandContext(ctx, "rg", args...)` — args は `[]string`、コマンド名ハードコード
- `search/fd.go:22`: `exec.CommandContext(ctx, "fd", args...)` — 同上

### ターミナル描画
- `tui/model.go:145`: `model.View()` の返り値 — 全描画の終着点
  - 内部で `lipgloss.NewStyle().Render(result.FileName)` 呼び出し → サニタイズ無し [要確認]
  - `chroma` 経由の preview 出力 → chroma自身がエスケープを付加するが元データの扱いは要調査

## Step 3: データフロー表

| 流入点 | 経由 | sink | 分類 | 既存防御 |
|--------|------|------|------|----------|
| `--query` | `searchEngine.Run` | `rg` の引数 | コマンドインジェクション | args配列渡し、`--`区切り ✓ |
| `rg --json` 出力 | `rgParser.Parse` → `model.Update` | `model.View` 描画 | ターミナルエスケープ注入 | **防御なし** [要対応] |
| `--preview-dir` | `previewer.ReadFile` | `os.Open` | パストラバーサル | `filepath.Clean` のみ、ベースディレクトリチェック無し [要確認] |

## Step 4: カバレッジ

| 項目 | 判定 | 根拠 |
|------|------|------|
| `--query` → `rg`引数 | Covered by tool | gosec G204 で検出可能、実装も安全 |
| `rg出力` → 描画 | Needs manual review | エスケープ注入は gosec では検出不可、Phase 3 で tui-checklist に沿って検証 |
| `--preview-dir` → `os.Open` | Needs manual review | パストラバーサル、防御の十分性を手動で確認 |
```
