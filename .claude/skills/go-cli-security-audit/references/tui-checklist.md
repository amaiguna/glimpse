# TUI特有のセキュリティチェック項目

Bubbletea/Lipgloss/bubbles スタックで作るTUIアプリケーション固有のセキュリティ観点。既製の静的解析ツール(gosec等)は**これらをほぼカバーしない**ので、手動レビューが必須の領域。

## 最重要: ターミナルエスケープシーケンス注入

### リスクの本質

ターミナルは `\x1b[` で始まる制御シーケンスによって、カーソル移動・色変更・画面クリア・**タイトルバー書き換え**・**クリップボードへの書き込み**(OSC 52)・**コマンド実行**(一部ターミナルの既知の脆弱性)等を受け付ける。

外部から来る文字列(ファイル名、ファイル内容、grep結果、ユーザー入力、コミットメッセージ、環境変数値など)に制御シーケンスが含まれていた場合、それを**そのままターミナルに描画する**と、攻撃者がターミナルの状態を操作できる。

過去の類似事例:
- `git log` で悪意あるコミットメッセージ経由の攻撃
- iTerm2 の OSC シーケンス経由のRCE(CVE-2019-9535)
- `less` や `more` の制御シーケンス展開問題

### TUI fuzzy finder における具体的な攻撃経路

fuzzy finder の場合、以下のすべてがエスケープ注入の攻撃面になりうる:

1. **ファイル名**: `fd` や `rg --files` の出力にはファイル名がそのまま含まれる。ファイル名に `\x1b[2J\x1b[H<偽のプロンプト>` のような制御シーケンスが入っていれば、画面を乗っ取れる可能性がある。
2. **grep結果の行内容**: `rg --json` で取得した `lines.text` フィールドの中身。
3. **preview に表示するファイル内容**: chroma でシンタックスハイライトする前の生データ。chromaは色付けのためにエスケープシーケンスを付加するが、元のファイル内容にエスケープが含まれていた場合の扱いは要確認。
4. **検索クエリのエコーバック**: ユーザー入力をそのまま描画するが、通常は制御文字を入力できないので相対的にはリスクが低い(ただしペースト入力なら可能)。

### チェックすべきコード箇所

```go
// ❌ 危険: 外部から来た文字列をそのままLipglossに流す
m.results = append(m.results, lipgloss.NewStyle().Render(filename))

// ❌ 危険: rgの出力をそのまま表示
fmt.Fprintln(output, line.Text)

// ⚠ 要確認: preview
preview := string(fileContent)  // エスケープシーケンスが含まれうる

// ✅ 推奨: 制御文字を除去またはエスケープしてから描画
safeName := sanitizeForTerminal(filename)
m.results = append(m.results, lipgloss.NewStyle().Render(safeName))
```

### サニタイズ実装例

```go
// 表示可能文字とUTF-8の範囲外を除去する基本実装
func sanitizeForTerminal(s string) string {
    var b strings.Builder
    for _, r := range s {
        switch {
        case r == '\t':
            b.WriteRune(r)
        case r < 0x20:            // 制御文字(タブ以外)
            b.WriteString(fmt.Sprintf("\\x%02x", r))
        case r == 0x7f:           // DEL
            b.WriteString("\\x7f")
        case unicode.IsControl(r):
            b.WriteString(fmt.Sprintf("\\u%04x", r))
        default:
            b.WriteRune(r)
        }
    }
    return b.String()
}
```

より厳密にやるなら、`golang.org/x/text/unicode/bidi` で双方向テキスト攻撃(Trojan Source, CVE-2021-42574)も考慮する。

### Lipgloss / Bubbletea の責任範囲

**Lipgloss は外部データのサニタイズを自動ではやらない。** スタイル適用(色、ボーダー、レイアウト)のみ。渡した文字列の中にエスケープシーケンスがあれば、そのままターミナルに出力される。サニタイズは**アプリケーション側の責任**。

Bubbletea の `View()` が返す string に含まれるエスケープシーケンスは全てターミナルに流れる。ここが事実上の最終 sink。

## ファイル preview の追加チェック項目

preview pane を持つTUI(fuzzy finder等)は、以下も確認:

### バイナリファイル

- 巨大なバイナリを読み込んでメモリ爆発しないか(サイズ上限)
- ヌル文字や不正UTF-8を含むファイル内容の扱い

### symlink

- preview先のファイルが symlink で `/etc/shadow` や `~/.ssh/id_rsa` を指していた場合どうなるか
- 読む前に `os.Lstat` で symlink 判定する、もしくはユーザーに明示的な許可を求める

### 特殊ファイル

- `/dev/zero`, `/dev/random` のようなブロックデバイスや無限ストリーム
- FIFO/socket(`os.Stat` の `Mode()` でチェック)
- 読み込みを `io.LimitReader` でサイズ制限する

## チェックリスト

TUI ツールをレビューする際の確認項目:

- [ ] 外部入力(ファイル名・ファイル内容・grep出力・環境変数)を描画する経路を全て列挙した
- [ ] 各描画経路でサニタイズまたはエスケープ処理が入っている
- [ ] サニタイズは制御文字・ANSI エスケープ・UTF-8 双方向制御文字を対象にしている
- [ ] preview で読むファイルにサイズ上限がある
- [ ] preview で symlink・特殊ファイルを適切にハンドリングしている
- [ ] `chroma` 等のハイライトライブラリに渡す前に入力が安全な形式になっている
- [ ] goroutine で非同期に描画する箇所で、データ競合(race)が `go test -race` で検出されない

## 参考情報

- Trojan Source (CVE-2021-42574): https://trojansource.codes/
- iTerm2 RCE (CVE-2019-9535)
- ANSIエスケープシーケンス一覧: https://en.wikipedia.org/wiki/ANSI_escape_code
