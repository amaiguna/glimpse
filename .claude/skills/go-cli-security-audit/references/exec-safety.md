# os/exec 安全性チェックリスト

Go の `os/exec` パッケージは設計上コマンドインジェクションに比較的強い(シェルを経由しないので)が、使い方を誤ると簡単に穴が開く。CLI/TUIツールは外部プロセス(`rg`, `fd`, `git` 等)を呼ぶ機会が多いので、ここは最優先で確認する。

## 安全な基本形

```go
// ✅ 推奨: 引数は配列で渡す。シェルを経由しない。
cmd := exec.CommandContext(ctx, "rg", "--json", query, "--", searchDir)
out, err := cmd.Output()
```

- `exec.CommandContext` を使う(`exec.Command` ではなく)。goroutineリーク防止のため必ず context を渡す。
- コマンド名はハードコード、ユーザー入力は**引数の要素として**渡す。
- `--` を使ってオプション終端を明示する(ユーザー入力が `-` で始まるケース対策)。

## アンチパターン一覧

### 1. sh -c でシェル経由

```go
// ❌ 危険: コマンドインジェクションそのもの
cmd := exec.Command("sh", "-c", fmt.Sprintf("rg %s", userQuery))
```

ユーザーが `'; rm -rf ~; echo '` のような入力を与えると即アウト。**絶対にやらない**。

どうしてもシェル機能(パイプ、リダイレクト)が必要な場合も、原則としては Go 側で `io.Pipe` や `cmd.Stdout` を繋いで実現する。

### 2. 引数を文字列連結して組み立てる

```go
// ❌ 危険: 一見 exec.Command でも、文字列構築時点でアウト
args := "rg --json " + userQuery
cmd := exec.Command("sh", "-c", args)
```

### 3. ユーザー入力をコマンド名に

```go
// ❌ 危険: 任意コマンド実行
cmd := exec.Command(userProvidedTool, "--version")
```

ユーザーが `/bin/sh` や悪意あるバイナリのパスを指定できる。コマンド名はハードコードするか、ホワイトリストで検証する。

### 4. PATHの未検証利用

```go
// ⚠ 要注意: PATH環境変数に悪意あるディレクトリが含まれていると、攻撃者の rg が実行される
cmd := exec.Command("rg", args...)
```

対策:

```go
// ✅ 絶対パスで指定、または LookPath で事前解決
rgPath, err := exec.LookPath("rg")
if err != nil {
    return fmt.Errorf("rg not found in PATH: %w", err)
}
// rgPath が意図したパス(例: /usr/bin/rg)かチェックしたい場合はここで検証
cmd := exec.CommandContext(ctx, rgPath, args...)
```

Go 1.19以降、`exec.LookPath` はカレントディレクトリの `.` を PATH に含まない仕様になったが、古いGoバージョンでは注意。

### 5. 引数に `-` 始まりのユーザー入力

```go
// ⚠ 要注意: ユーザーが "--exec=rm -rf /" と入力したら?
cmd := exec.Command("grep", pattern, filename)
```

`pattern` や `filename` がユーザー入力なら、`--` で区切ること:

```go
// ✅
cmd := exec.Command("grep", "--", pattern, filename)
```

ツールによって `--` のサポート有無が異なるので、呼び出すツールのドキュメントで確認する。`rg`, `fd`, `grep`, `git` などはサポートしている。

### 6. 環境変数の未制御

```go
// ⚠ 要注意: 親プロセスの全環境変数が引き継がれる
cmd := exec.Command("rg", args...)
cmd.Run()
```

機密情報を含む環境変数が意図せず渡るリスク。また、`LD_PRELOAD` や `GIT_SSH_COMMAND` のような**挙動を変える**環境変数が汚染されている可能性も。

```go
// ✅ 必要な環境変数だけ明示的に渡す
cmd := exec.Command("rg", args...)
cmd.Env = []string{
    "PATH=" + os.Getenv("PATH"),
    "HOME=" + os.Getenv("HOME"),
    "LANG=" + os.Getenv("LANG"),
}
```

ツールによっては `HOME` が無いと設定ファイルを読み込めないなどの副作用もあるので、必要なものを洗い出してから切る。

### 7. 作業ディレクトリの未設定

```go
// ⚠ カレントディレクトリで動く。意図しないディレクトリの可能性。
cmd := exec.Command("git", "status")
```

特にCLIツールが `os.Chdir` を使ったり、ユーザーが任意のディレクトリを指定できる場合、明示的に `cmd.Dir` を設定する。

```go
cmd := exec.Command("git", "status")
cmd.Dir = validatedProjectDir
```

## stdout/stderr/stdin の扱い

### 標準出力のサイズ制限

`cmd.Output()` は全部メモリに読む。巨大な出力で OOM する可能性。

```go
// ✅ パイプで受けて io.LimitReader で制限
stdout, _ := cmd.StdoutPipe()
cmd.Start()
limited := io.LimitReader(stdout, 100*1024*1024) // 100MB上限
data, err := io.ReadAll(limited)
cmd.Wait()
```

### stderr を捨てない

デバッグ時やエラー分析時にstderrを見られるようにしておく。エラーの詳細を失うと、攻撃検出も難しくなる。

```go
var stderr bytes.Buffer
cmd.Stderr = &stderr
if err := cmd.Run(); err != nil {
    return fmt.Errorf("rg failed: %w (stderr: %s)", err, stderr.String())
}
```

### タイムアウト

`context.WithTimeout` と `exec.CommandContext` を組み合わせて、暴走する外部プロセスを殺せるようにする:

```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "rg", args...)
```

## チェックリスト

- [ ] `sh -c` や `bash -c` を使っていない
- [ ] コマンド名はハードコードまたはホワイトリスト済み
- [ ] 引数は文字列結合ではなく配列要素として渡されている
- [ ] ユーザー入力を含む引数の前に `--` が入っている(ツールがサポートする場合)
- [ ] `exec.CommandContext` で context + timeout を設定している
- [ ] `cmd.Dir` が意図したディレクトリに設定されている(必要な場合)
- [ ] 環境変数の範囲を制限している、または必要性を確認済み
- [ ] stdout サイズに上限がある(巨大出力でもOOMしない)
- [ ] stderr をエラー時に保持している
- [ ] `exec.LookPath` の結果をチェックしている(PATH汚染対策)
