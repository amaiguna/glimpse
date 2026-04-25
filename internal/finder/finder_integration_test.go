//go:build integration

package finder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M-3 回帰: ListFiles が context を受け取り、キャンセル/タイムアウトを
// fd/rg プロセスに伝播させること。

func TestListFilesContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListFiles(ctx)
	require.Error(t, err, "キャンセル済み ctx では ListFiles はエラーを返す")
}

func TestListFilesNormalContext(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("y"), 0644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(wd)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	items, err := ListFiles(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}
