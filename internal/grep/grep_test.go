package grep

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
