package grep

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// Match はrgの検索結果から抽出した1行分のマッチ情報を表す。
type Match struct {
	File string // マッチしたファイルのパス
	Line int    // マッチした行番号（1始まり）
	Text string // マッチした行の全文（末尾改行なし）
}

// rgMessage は rg --json が出力する1行分のJSONメッセージ。
// Type は "begin"（ファイル検索開始）, "match"（マッチ行）, "end"（ファイル検索終了）, "summary"（全体統計）のいずれか。
// Data の中身は Type によって構造が異なるため json.RawMessage で遅延パースする。
type rgMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// rgMatch は rgMessage の Type が "match" のときの Data の構造。
type rgMatch struct {
	// Path はマッチしたファイルのパス情報。
	Path struct {
		Text string `json:"text"`
	} `json:"path"`
	// Lines はマッチした行の全文。末尾に改行を含む。
	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`
	// LineNumber はファイル内の行番号（1始まり）。
	LineNumber int `json:"line_number"`
	// Submatches は行内でパターンにマッチした部分のリスト。
	// Start/End は行内のバイト位置で、ハイライト表示に使える。
	Submatches []struct {
		Match struct {
			Text string `json:"text"` // マッチした文字列
		} `json:"match"`
		Start int `json:"start"` // 行内の開始バイト位置
		End   int `json:"end"`   // 行内の終了バイト位置
	} `json:"submatches"`
}

// Search は rg --json を実行し、結果をパースして返す。
// マッチなし（終了コード1）の場合は nil, nil を返す。
// ctx のキャンセル/タイムアウトは rg プロセスに伝播し、呼び出し側は
// 古いデバウンスをキャンセルして stdout の溜め込みを防げる。
func Search(ctx context.Context, pattern string) ([]Match, error) {
	cmd := exec.CommandContext(ctx, "rg", "--json", pattern)
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return ParseRgJSON(string(out))
}

// ParseRgJSON は rg --json の出力文字列をパースし、マッチ行のみを抽出する。
// 不正なJSONやmatch以外のメッセージ（begin, end, summary）はスキップする。
func ParseRgJSON(output string) ([]Match, error) {
	var matches []Match
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		var msg rgMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type != "match" {
			continue
		}
		var data rgMatch
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			continue
		}
		if data.Path.Text == "" {
			continue
		}
		matches = append(matches, Match{
			File: data.Path.Text,
			Line: data.LineNumber,
			Text: strings.TrimRight(data.Lines.Text, "\n"),
		})
	}
	return matches, nil
}
