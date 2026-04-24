# Fuzz Target 選定とテンプレート

Go 1.18+ の `testing.F` を使った fuzz テストは、攻撃者が制御できる入力を扱うパーサー系コードに対して非常に効果的。ここでは fuzz 対象の選び方と、典型的なtargetのスケルトンを示す。

## fuzz 対象として有望な関数の特徴

以下のいずれかに該当する関数は fuzz 対象として優先度が高い:

1. **外部からの非構造化バイト列を構造化データに変換する**
    - クエリパーサー、URL/パスパーサー、設定ファイルパーサー
    - コマンドライン引数の解釈(独自実装している場合)
2. **外部プロセスの出力をデシリアライズする**
    - `rg --json` の出力、`git` のポーセリン出力
    - JSON/YAML/TOML等のデコード後に独自処理を挟む箇所
3. **ユーザー入力から SQL やパスやコマンド引数を組み立てる**
    - エスケープ処理の正当性を検証できる
4. **パーサー以外でも、外部入力がメモリ確保量・再帰深度を左右する**
    - 正規表現マッチ、文字列処理、ツリー構築等

逆に、**純粋なビジネスロジック**(計算関数等)で外部入力が介在しないものは、fuzz の ROI は低い。通常のユニットテストで十分。

## 基本テンプレート

```go
package mypackage

import (
    "testing"
)

func FuzzParseQuery(f *testing.F) {
    // seed corpus: 正常ケース、境界ケース、既知のバグケース
    seeds := []string{
        "",
        "hello",
        "hello world",
        "\"quoted string\"",
        "\\escaped",
        "multibyte: 日本語",
        string([]byte{0xff, 0xfe}),  // 不正UTF-8
    }
    for _, s := range seeds {
        f.Add(s)
    }

    f.Fuzz(func(t *testing.T, input string) {
        result, err := ParseQuery(input)
        if err != nil {
            // エラーはOK。ただしpanicはNG。
            return
        }
        // プロパティチェック: 成功した場合の不変条件を検証
        if result == nil {
            t.Fatal("ParseQuery returned nil without error")
        }
        // 例: 出力が何らかの不変条件を満たすべき
        if len(result.Terms) == 0 && input != "" {
            t.Fatalf("empty result from non-empty input: %q", input)
        }
    })
}
```

## 主要な検証パターン

### パターン1: パニックしないこと

最小限の検証。どんな入力でもクラッシュしないことを確認する。

```go
f.Fuzz(func(t *testing.T, input string) {
    _, _ = ParseQuery(input)
    // panicしなければOK
})
```

### パターン2: ラウンドトリップ

シリアライズ→デシリアライズで元に戻るか。

```go
f.Fuzz(func(t *testing.T, input string) {
    parsed, err := Parse(input)
    if err != nil {
        return
    }
    reserialized := parsed.String()
    parsed2, err := Parse(reserialized)
    if err != nil {
        t.Fatalf("failed to re-parse: %q -> %q", input, reserialized)
    }
    if !parsed.Equals(parsed2) {
        t.Fatal("roundtrip failed")
    }
})
```

### パターン3: 2つの実装の等価性

リファクタリング時や、実装の正当性検証に有効。

```go
f.Fuzz(func(t *testing.T, input string) {
    r1 := OldImpl(input)
    r2 := NewImpl(input)
    if !reflect.DeepEqual(r1, r2) {
        t.Fatalf("implementations differ on %q", input)
    }
})
```

### パターン4: 不変条件

成功時の出力が満たすべき性質を検証。

```go
f.Fuzz(func(t *testing.T, path string) {
    cleaned, err := SanitizePath(path)
    if err != nil {
        return
    }
    // サニタイズ後のパスは必ずベースディレクトリ配下に収まっているべき
    if !strings.HasPrefix(cleaned, baseDir+"/") && cleaned != baseDir {
        t.Fatalf("escape from baseDir: %q -> %q", path, cleaned)
    }
})
```

## 典型的なtarget: CLI/TUI ツール特有

### 検索クエリパーサー

fuzzy finder の正規表現・マッチング関数に対して:

```go
func FuzzFuzzyMatch(f *testing.F) {
    seeds := []struct {
        pattern, text string
    }{
        {"", ""},
        {"a", "abc"},
        {".*", "anything"},
        {strings.Repeat("a", 1000), "a"},  // 病的入力
    }
    for _, s := range seeds {
        f.Add(s.pattern, s.text)
    }
    f.Fuzz(func(t *testing.T, pattern, text string) {
        // タイムアウト付きで実行するためgoroutineで包む
        done := make(chan struct{})
        go func() {
            defer close(done)
            _ = FuzzyMatch(pattern, text)
        }()
        select {
        case <-done:
            // OK
        case <-time.After(1 * time.Second):
            t.Fatalf("FuzzyMatch took too long: pattern=%q text=%q", pattern, text)
        }
    })
}
```

### ripgrep JSON出力パーサー

```go
func FuzzParseRgJSON(f *testing.F) {
    // 実際の rg --json 出力サンプルをseedに
    f.Add(`{"type":"match","data":{"path":{"text":"foo.go"},"lines":{"text":"hello"},"line_number":1}}`)
    f.Add(`{"type":"begin","data":{"path":{"text":"foo.go"}}}`)
    f.Add(``)
    f.Add(`{`)  // 不完全

    f.Fuzz(func(t *testing.T, input string) {
        _, _ = ParseRgLine(input)
        // panicしないこと
    })
}
```

### ファイルパスのサニタイズ

```go
func FuzzSanitizePath(f *testing.F) {
    seeds := []string{
        "foo.txt",
        "../etc/passwd",
        "foo/../bar",
        "foo/./bar",
        "//double/slash",
        "\x00null",
        strings.Repeat("../", 1000),
    }
    for _, s := range seeds {
        f.Add(s)
    }
    f.Fuzz(func(t *testing.T, input string) {
        cleaned, err := SanitizePath(baseDir, input)
        if err != nil {
            return
        }
        // ベースディレクトリ外に出ていないこと
        abs, _ := filepath.Abs(cleaned)
        if !strings.HasPrefix(abs, baseDir) {
            t.Fatalf("path escape: %q -> %q", input, cleaned)
        }
    })
}
```

## 実行と CI 統合

### 手元での実行

```bash
# デフォルトはシード corpus のみ実行
go test -fuzz=FuzzParseQuery -fuzztime=30s ./...

# CIでは時間を決めて継続実行
go test -fuzz=FuzzParseQuery -fuzztime=5m ./...
```

### corpus の管理

- 失敗した入力は `testdata/fuzz/FuzzXxx/` に自動保存される。これはリポジトリにコミットする。
- 手で追加したい seed ケースは `f.Add(...)` で明示。

### CI 統合の考え方

- 通常のテストでは `f.Add` の seed のみ実行される(`go test ./...` だけでOK)。
- 本格的な fuzz は別ジョブで長時間走らせる(nightlyや週次)。
- 失敗したケースが発見されたら、testdata にcommitして回帰テスト化する。

## 補足: 既に見つかっている制約

- `testing.F` は並列実行できないので、並列性が重要な fuzz は自前で goroutine を管理する必要がある。
- メモリ不足で死にやすいので、`go test -fuzz=X -fuzzminimizetime=10s` などで制約する。
