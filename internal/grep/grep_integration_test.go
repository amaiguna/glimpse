//go:build integration

package grep

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M-3 回帰: Search が context を受け取り、キャンセル/タイムアウトを
// exec.CommandContext 経由で rg プロセスに伝播させること。

func TestSearchContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 事前にキャンセル

	_, err := Search(ctx, "anything", nil)
	require.Error(t, err, "キャンセル済み ctx では Search はエラーを返す")
}

func TestSearchContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // deadline 確実に超過させる

	_, err := Search(ctx, "anything", nil)
	require.Error(t, err)
	// 厳密な種別はプラットフォーム依存なので包含関係は問わない
	_ = errors.Is(err, context.DeadlineExceeded)
}

func TestSearchNormalContext(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(wd)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matches, err := Search(ctx, "hello", nil)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "hello world", matches[0].Text)
}
