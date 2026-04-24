# Phase 1: 自動静的解析ツール

Go製プロジェクトに対して機械的に実行できるセキュリティ検査ツール群。実行順序、コマンド例、出力の読み方、ツール間の守備範囲の重なりを記載する。

## 実行順序の推奨

1. `go mod tidy` → `go.sum` の整合性確認
2. `govulncheck` — 既知CVEを実コールパスで絞り込み
3. `osv-scanner` — 依存関係のCVE広めにチェック(govulncheckとの差分を確認する意図)
4. `gosec` — 典型的アンチパターン
5. `staticcheck` — 一般バグ(セキュリティに間接的に効く)
6. `go test -race ./...` — レース検出

全ツール合わせても通常は数分で完了する。CI/CDに組み込む場合は、少なくとも `govulncheck` と `gosec` は必須級。

## govulncheck

Go公式の脆弱性スキャナ。単なる依存関係のCVEマッチングではなく、**実際のコールパスで到達する脆弱性のみ**を報告してくれる点が強み(依存ライブラリにCVEがあっても、その関数を呼んでいなければ報告されない)。

### インストールと実行

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

### 出力の読み方

- `Vulnerability #N` の下に **Call stacks in your code** が表示されるものは、**実際に到達可能な脆弱性**。優先対応。
- 依存関係にはCVEがあるが、Call stacksに該当しない場合は影響なし。ただし将来のリファクタで呼び出すリスクはあるので記録しておく。
- `Standard library` のCVEは Go バージョンアップで解決することが多い。

### 対応方針

- 影響ありと判定されたら、依存ライブラリのバージョンアップ(`go get -u <pkg>`)か、Goのバージョンアップ。
- 該当関数を呼ばないよう回避するのは最後の手段。

## osv-scanner

Google製。OSV database に基づいて依存関係全体のCVEをチェック。govulncheckより広範囲をカバーするが、Call stack解析はしないので**ノイズが多め**。

### インストールと実行

```bash
go install github.com/google/osv-scanner/cmd/osv-scanner@latest
osv-scanner --lockfile go.mod
```

### 出力の読み方

- govulncheck で拾われなかったものが出てきたら、それが「依存関係にCVEはあるが実コールパスには載っていない」もの。基本的に即時対応不要だが記録する。
- govulncheck と重複する指摘は govulncheck の判定を優先。

### 使い分け

govulncheck を CI/CD のブロッカーに、osv-scanner を広めの監視に、という役割分担が実用的。

## gosec

Go コードの典型的なセキュリティアンチパターンを検出する静的解析ツール。

### インストールと実行

```bash
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...
```

### 主な検出項目

- `G101`: ハードコードされた認証情報
- `G104`: エラーチェック漏れ(攻撃検出を見逃す原因になる)
- `G107`: 外部入力から構築されたURLへのHTTPリクエスト
- `G201/G202`: SQLインジェクション
- `G204`: 外部入力を含む `exec.Command` 呼び出し ← **CLI/TUIツールでは特に重要**
- `G301/G302/G304/G306`: ファイル・ディレクトリのパーミッション、パストラバーサル
- `G401/G402/G403/G404`: 弱い暗号・TLS設定・不十分な乱数
- `G501-G505`: 非推奨の暗号アルゴリズム使用

### 誤検知の扱い

gosec は比較的誤検知が多いツール。以下のパターンは典型的な誤検知:

- `G204` でコマンド名がハードコードされている場合(例: `exec.Command("rg", userInput...)` の `"rg"` 部分には攻撃者は触れない)→ 引数側が問題なければOK
- `G304` で `filepath.Clean` 後の安全なパスに対する指摘 → ベースディレクトリ制約を実装していれば問題なし
- テストコード内の `G101` → 大抵は無視してよい

誤検知と判断した場合は、コメント `// #nosec G204 -- <理由>` で抑制できる。理由は必ず書くこと(将来のレビューワーのため)。

## staticcheck

セキュリティ専門ではないが、一般バグを通じて間接的にセキュリティに効くことが多い。

### 実行

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...
```

### セキュリティに関係する指摘例

- `SA1019`: 非推奨API使用(古い暗号APIなど)
- `SA4006`: 未使用の値(エラー処理漏れの可能性)
- `SA5011`: nil pointer dereference 可能性

## go test -race

```bash
go test -race ./...
```

データレースは直接の脆弱性ではないが、TOCTOU起因のセキュリティバグや状態破壊の温床になる。TUIアプリはgoroutineを多用するので特に重要。

## ツール横断の注意

- **`errcheck`**: エラー処理漏れを検出する専用ツール。`gosec G104` と重複するが、より徹底したい場合に使用。
- **`semgrep`**: Go用ルールセットもあるが、gosec/staticcheckで大半カバーできるので、独自ルールを書きたい場合に検討。
- すべてのツールは CI/CD に組み込み、PR時点で自動実行することを推奨。特に `govulncheck` は定期実行も意味がある(新規CVEが公開されるたびに結果が変わるため)。

## 出力のまとめ方

Phase 1 完了時点で、以下を整理してユーザーに報告する:

1. 各ツールの実行コマンドと所要時間
2. 発見された真の問題(誤検知を除いた後)を優先度付きで一覧化
3. 対応方針(バージョンアップ、コード修正、誤検知として抑制)
4. Phase 2 に持ち越す疑義(ツールでは判定不能なもの、深い分析が必要なもの)
