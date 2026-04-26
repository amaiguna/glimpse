package finder

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

// I-1 回帰: readLimited は max バイトまで読み、超過時は ErrOutputTooLarge を返す。
// 巨大リポでの fd / rg --files の出力が OOM 化しないための安全網。
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

// readLimited は Reader からのエラーをそのまま伝搬する。
func TestReadLimitedPropagatesReaderError(t *testing.T) {
	sentinel := errors.New("read failed")
	r := iotest.ErrReader(sentinel)
	_, err := readLimited(r, 1024)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinel) || strings.Contains(err.Error(), sentinel.Error()),
		"unexpected error: %v", err)
}

// L-2 回帰: fd / rg は起動時に LookPath で絶対パス解決されていること。

func TestFdBinaryResolvedToAbsolutePath(t *testing.T) {
	if fdBinary == "" {
		t.Skip("fd がインストールされていない環境ではスキップ")
	}
	assert.True(t, filepath.IsAbs(fdBinary),
		"fd は絶対パスで解決されているべき: got %q", fdBinary)
}

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

// #008: CmdError は非0終了時の構造化情報を保持し、Error() に stderr を含める。
func TestCmdErrorErrorIncludesStderr(t *testing.T) {
	inner := errors.New("exit status 2")
	e := &CmdError{ExitCode: 2, Stderr: "fd: invalid value", Err: inner}
	got := e.Error()
	assert.Contains(t, got, "exit status 2")
	assert.Contains(t, got, "fd: invalid value")
}

func TestCmdErrorErrorWithoutStderr(t *testing.T) {
	inner := errors.New("exit status 1")
	e := &CmdError{ExitCode: 1, Stderr: "", Err: inner}
	assert.Equal(t, "exit status 1", e.Error())
}

func TestCmdErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &CmdError{ExitCode: 2, Stderr: "x", Err: inner}
	assert.True(t, errors.Is(e, inner))
}

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
	cmd := exec.Command("/bin/sh", "-c", "echo 'fd: not a directory' >&2; exit 2")
	_, err := runWithLimit(cmd, 1024)
	require.Error(t, err)
	var cmdErr *CmdError
	require.ErrorAs(t, err, &cmdErr)
	assert.Equal(t, 2, cmdErr.ExitCode)
	assert.Contains(t, cmdErr.Stderr, "fd: not a directory")
}

// #008: stderr が膨大でも MaxCmdStderrSize で切り詰められる。
func TestRunWithLimitStderrIsBounded(t *testing.T) {
	requireShell(t)
	script := `awk 'BEGIN{ for(i=0;i<1048576;i++) printf "x"; print "" > "/dev/stderr" }' >&2; exit 2`
	cmd := exec.Command("/bin/sh", "-c", script)
	_, err := runWithLimit(cmd, 1024)
	require.Error(t, err)
	var cmdErr *CmdError
	require.ErrorAs(t, err, &cmdErr)
	assert.LessOrEqual(t, len(cmdErr.Stderr), MaxCmdStderrSize)
}

func TestRunWithLimitNoErrorOnSuccess(t *testing.T) {
	requireShell(t)
	cmd := exec.Command("/bin/sh", "-c", "echo hello")
	out, err := runWithLimit(cmd, 1024)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

// #008: fd 失敗時に rg にフォールバックする既存挙動が CmdError 経由でも保たれる。
func TestListFilesFallsBackFromFdToRg(t *testing.T) {
	requireShell(t)
	origFd, origRg := fdBinary, rgBinary
	t.Cleanup(func() {
		fdBinary = origFd
		rgBinary = origRg
	})

	dir := t.TempDir()
	fakeFd := filepath.Join(dir, "fd")
	require.NoError(t, writeExecutable(fakeFd, "#!/bin/sh\necho 'fd: explode' >&2\nexit 2\n"))
	fakeRg := filepath.Join(dir, "rg")
	require.NoError(t, writeExecutable(fakeRg, "#!/bin/sh\nprintf 'a.go\\nb.go\\n'\n"))
	fdBinary = fakeFd
	rgBinary = fakeRg

	files, err := ListFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a.go", "b.go"}, files)
}

// #008: rg --files の非0終了は CmdError として上位に伝搬する。
func TestListFilesPropagatesCmdErrorFromRg(t *testing.T) {
	requireShell(t)
	origFd, origRg := fdBinary, rgBinary
	t.Cleanup(func() {
		fdBinary = origFd
		rgBinary = origRg
	})

	dir := t.TempDir()
	fakeRg := filepath.Join(dir, "rg")
	require.NoError(t, writeExecutable(fakeRg, "#!/bin/sh\necho 'rg: io error' >&2\nexit 2\n"))
	fdBinary = ""
	rgBinary = fakeRg

	_, err := ListFiles(context.Background())
	require.Error(t, err)
	var cmdErr *CmdError
	require.ErrorAs(t, err, &cmdErr)
	assert.Equal(t, 2, cmdErr.ExitCode)
	assert.Contains(t, cmdErr.Stderr, "rg: io error")
}

func writeExecutable(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o755)
}

// fd / rg の両方が見つからない場合、ListFiles は明示的なエラーを返す。
func TestListFilesReturnsErrorWhenBothBinariesMissing(t *testing.T) {
	origFd, origRg := fdBinary, rgBinary
	fdBinary = ""
	rgBinary = ""
	defer func() {
		fdBinary = origFd
		rgBinary = origRg
	}()

	_, err := ListFiles(context.Background())
	require.Error(t, err)
}
