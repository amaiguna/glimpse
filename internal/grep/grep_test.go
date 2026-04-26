package grep

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeExecutable は Search 経由のサブプロセス起動テストで使う使い捨てスクリプトを書き出す。
func writeExecutable(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o755)
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func TestParseRgJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Match
		wantErr bool
	}{
		{
			name:  "single match",
			input: `{"type":"match","data":{"path":{"text":"main.go"},"lines":{"text":"func main()\n"},"line_number":10,"submatches":[{"match":{"text":"main"},"start":5,"end":9}]}}`,
			want: []Match{
				{File: "main.go", Line: 10, Text: "func main()"},
			},
		},
		{
			name: "multiple matches",
			input: `{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"foo\n"},"line_number":1,"submatches":[]}}
{"type":"match","data":{"path":{"text":"b.go"},"lines":{"text":"bar\n"},"line_number":5,"submatches":[]}}`,
			want: []Match{
				{File: "a.go", Line: 1, Text: "foo"},
				{File: "b.go", Line: 5, Text: "bar"},
			},
		},
		{
			name:  "non-match types are skipped",
			input: `{"type":"begin","data":{"path":{"text":"main.go"}}}`,
			want:  nil,
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "invalid JSON is skipped",
			input: `{not valid json}`,
			want:  nil,
		},
		{
			name: "match type with invalid data is skipped",
			input: `{"type":"match","data":[1,2,3]}
{"type":"match","data":{"path":{"text":""},"lines":{"text":"x"},"line_number":1,"submatches":[]}}`,
			want: nil,
		},
		{
			name: "mixed valid and invalid lines",
			input: `{"type":"begin","data":{"path":{"text":"main.go"}}}
{broken line
{"type":"match","data":{"path":{"text":"main.go"},"lines":{"text":"hello\n"},"line_number":3,"submatches":[]}}
{"type":"end","data":{"path":{"text":"main.go"}}}`,
			want: []Match{
				{File: "main.go", Line: 3, Text: "hello"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRgJSON(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// I-1 回帰: readLimited は max バイトまで読み、超過時は ErrOutputTooLarge を返す。
// rg --files / rg --json の出力が巨大な場合に OOM 化させない安全網。
func TestReadLimited(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		max     int64
		wantOut string
		wantErr error
	}{
		{name: "empty input", input: "", max: 10, wantOut: ""},
		{name: "within limit", input: "hello", max: 10, wantOut: "hello"},
		{name: "exactly at limit", input: "0123456789", max: 10, wantOut: "0123456789"},
		{name: "one byte over", input: "0123456789X", max: 10, wantErr: ErrOutputTooLarge},
		{name: "much larger", input: strings.Repeat("a", 10_000), max: 100, wantErr: ErrOutputTooLarge},
		{name: "zero limit empty input", input: "", max: 0, wantOut: ""},
		{name: "zero limit nonempty", input: "x", max: 0, wantErr: ErrOutputTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readLimited(strings.NewReader(tt.input), tt.max)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), "expected %v, got %v", tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, string(got))
		})
	}
}

// readLimited は Reader からのエラーをそのまま伝搬する（context cancel など）。
func TestReadLimitedPropagatesReaderError(t *testing.T) {
	sentinel := errors.New("read failed")
	r := iotest.ErrReader(sentinel)
	_, err := readLimited(r, 1024)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinel) || strings.Contains(err.Error(), sentinel.Error()),
		"unexpected error: %v", err)
}

// L-2 回帰: rg バイナリは起動時に LookPath で絶対パス解決されていること。
// 相対名 "rg" のままだと実行時 PATH 順序次第で差し替えが効いてしまうため。
func TestRgBinaryResolvedToAbsolutePath(t *testing.T) {
	if rgBinary == "" {
		t.Skip("rg がインストールされていない環境ではスキップ")
	}
	assert.True(t, filepath.IsAbs(rgBinary),
		"rg は絶対パスで解決されているべき: got %q", rgBinary)
}

// L-3 回帰: whitelistedEnv は PATH/HOME/LANG/LC_* のみを通し、
// LD_PRELOAD や GIT_SSH_COMMAND などの危険な/無関係な env を落とすこと。
func TestWhitelistedEnv(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/home/user")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LD_PRELOAD", "/malicious.so")
	t.Setenv("GIT_SSH_COMMAND", "evil")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	m := envMap(whitelistedEnv())

	assert.Equal(t, "/usr/bin", m["PATH"])
	assert.Equal(t, "/home/user", m["HOME"])
	assert.Equal(t, "en_US.UTF-8", m["LANG"])
	assert.Equal(t, "en_US.UTF-8", m["LC_ALL"])
	assert.NotContains(t, m, "LD_PRELOAD", "LD_PRELOAD は継承されてはならない")
	assert.NotContains(t, m, "GIT_SSH_COMMAND")
	assert.NotContains(t, m, "AWS_SECRET_ACCESS_KEY", "クレデンシャル系は継承されてはならない")
}

// rg が PATH 上で見つからないケースで Search は明示的なエラーを返す。
// 実プロセスを起動する前の早期 fail を保証する（integration 扱いではなく unit で実行可）。
func TestSearchReturnsErrorWhenBinaryMissing(t *testing.T) {
	orig := rgBinary
	rgBinary = ""
	defer func() { rgBinary = orig }()

	_, err := Search(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rg")
}

// #008: CmdError は非0終了時の構造化情報を保持し、Error() に stderr を
// 含めることで「exit status 2」だけの不可解なメッセージを解消する。
func TestCmdErrorErrorIncludesStderr(t *testing.T) {
	inner := errors.New("exit status 2")
	e := &CmdError{ExitCode: 2, Stderr: "regex parse error: foo", Err: inner}
	got := e.Error()
	assert.Contains(t, got, "exit status 2")
	assert.Contains(t, got, "regex parse error: foo")
}

// #008: stderr が空ならラップされた err と同等のメッセージを返す。
func TestCmdErrorErrorWithoutStderr(t *testing.T) {
	inner := errors.New("exit status 1")
	e := &CmdError{ExitCode: 1, Stderr: "", Err: inner}
	assert.Equal(t, "exit status 1", e.Error())
}

// #008: errors.Unwrap で内側の err を取り出せる（errors.Is/As の連鎖を保つ）。
func TestCmdErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &CmdError{ExitCode: 2, Stderr: "x", Err: inner}
	assert.True(t, errors.Is(e, inner))
}

// requireShell は /bin/sh を要する小さなサブプロセス起動テストの前提。
// Windows ではスキップ、それ以外で sh が無ければエラーで落とす。
func requireShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows では /bin/sh によるテストをスキップ")
	}
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skipf("/bin/sh が見つからないためスキップ: %v", err)
	}
}

// #008: runWithLimit は子プロセスの stderr を捕捉し、非0終了時に CmdError でラップする。
func TestRunWithLimitCapturesStderrOnNonZeroExit(t *testing.T) {
	requireShell(t)
	cmd := exec.Command("/bin/sh", "-c", "echo 'regex parse error: oops' >&2; exit 2")
	_, err := runWithLimit(cmd, 1024)
	require.Error(t, err)
	var cmdErr *CmdError
	require.ErrorAs(t, err, &cmdErr)
	assert.Equal(t, 2, cmdErr.ExitCode)
	assert.Contains(t, cmdErr.Stderr, "regex parse error: oops")
}

// #008: stderr が膨大でも MaxCmdStderrSize で切り詰められ、メモリが膨らまない。
func TestRunWithLimitStderrIsBounded(t *testing.T) {
	requireShell(t)
	// 1MB 相当の stderr を吐き出して非0終了
	script := `awk 'BEGIN{ for(i=0;i<1048576;i++) printf "x"; print "" > "/dev/stderr" }' >&2; exit 2`
	cmd := exec.Command("/bin/sh", "-c", script)
	_, err := runWithLimit(cmd, 1024)
	require.Error(t, err)
	var cmdErr *CmdError
	require.ErrorAs(t, err, &cmdErr)
	assert.LessOrEqual(t, len(cmdErr.Stderr), MaxCmdStderrSize,
		"stderr 取り込みは MaxCmdStderrSize 以下に抑える必要がある")
}

// #008: 正常終了時は CmdError を返さない（既存挙動の回帰防止）。
func TestRunWithLimitNoErrorOnSuccess(t *testing.T) {
	requireShell(t)
	cmd := exec.Command("/bin/sh", "-c", "echo hello")
	out, err := runWithLimit(cmd, 1024)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

// #008 + 既存 Search の回帰: rg 相当の exit code 1（マッチなし）は CmdError ではなく
// nil, nil として扱われ続ける必要がある。
func TestSearchExitCode1IsNoMatch(t *testing.T) {
	requireShell(t)
	orig := rgBinary
	t.Cleanup(func() { rgBinary = orig })

	// rg のフリをして exit 1 を返すスクリプトを作る
	dir := t.TempDir()
	fake := filepath.Join(dir, "rg")
	err := writeExecutable(fake, "#!/bin/sh\nexit 1\n")
	require.NoError(t, err)
	rgBinary = fake

	matches, err := Search(context.Background(), "anything")
	require.NoError(t, err)
	assert.Nil(t, matches)
}

// #008: rg が exit code 2 + stderr を返すケースで、Search はそれを CmdError として伝搬する。
func TestSearchExitCode2PropagatesCmdError(t *testing.T) {
	requireShell(t)
	orig := rgBinary
	t.Cleanup(func() { rgBinary = orig })

	dir := t.TempDir()
	fake := filepath.Join(dir, "rg")
	err := writeExecutable(fake, "#!/bin/sh\necho 'regex parse error: unclosed character class' >&2\nexit 2\n")
	require.NoError(t, err)
	rgBinary = fake

	_, err = Search(context.Background(), "[")
	require.Error(t, err)
	var cmdErr *CmdError
	require.ErrorAs(t, err, &cmdErr)
	assert.Equal(t, 2, cmdErr.ExitCode)
	assert.Contains(t, cmdErr.Stderr, "regex parse error")
}

func FuzzParseRgJSON(f *testing.F) {
	// シード: rgが実際に出力する形式と、壊れた入力のバリエーション
	f.Add(`{"type":"match","data":{"path":{"text":"main.go"},"lines":{"text":"func main()\n"},"line_number":10,"submatches":[]}}`)
	f.Add(`{"type":"begin","data":{"path":{"text":"main.go"}}}`)
	f.Add(``)
	f.Add(`{invalid`)
	f.Add(`{"type":"match","data":null}`)
	f.Add(strings.Repeat(`{"type":"match","data":{"path":{"text":"x"},"lines":{"text":"y"},"line_number":1,"submatches":[]}}`+"\n", 100))

	f.Fuzz(func(t *testing.T, input string) {
		// パニックせずに返ればOK — 戻り値の正しさは問わない
		matches, err := ParseRgJSON(input)
		if err != nil {
			return
		}
		for _, m := range matches {
			if m.File == "" {
				t.Error("match with empty File")
			}
			if m.Line < 0 {
				t.Errorf("negative line number: %d", m.Line)
			}
		}
	})
}
