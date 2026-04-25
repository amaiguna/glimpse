package finder

import (
	"context"
	"errors"
	"path/filepath"
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
