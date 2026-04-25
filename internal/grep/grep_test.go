package grep

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
			name:  "match type with invalid data is skipped",
			input: `{"type":"match","data":[1,2,3]}
{"type":"match","data":{"path":{"text":""},"lines":{"text":"x"},"line_number":1,"submatches":[]}}`,
			want:  nil,
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

func FuzzParseRgJSON(f *testing.F) {
	// シード: rgが実際に出力する形式と、壊れた入力のバリエーション
	f.Add(`{"type":"match","data":{"path":{"text":"main.go"},"lines":{"text":"func main()\n"},"line_number":10,"submatches":[]}}`)
	f.Add(`{"type":"begin","data":{"path":{"text":"main.go"}}}`)
	f.Add(``)
	f.Add(`{invalid`)
	f.Add(`{"type":"match","data":null}`)
	f.Add(strings.Repeat(`{"type":"match","data":{"path":{"text":"x"},"lines":{"text":"y"},"line_number":1,"submatches":[]}}` + "\n", 100))

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
