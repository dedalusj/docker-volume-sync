package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/stretchr/testify/require"
)

func TestSync_CopyDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	require.NoError(t, os.Mkdir(srcDir, 0755))
	require.NoError(t, os.Mkdir(dstDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "todelete.txt"), []byte("delete me"), 0644))

	s, err := New(context.Background())
	require.NoError(t, err)

	err = s.Sync(context.Background(), srcDir, dstDir)
	require.NoError(t, err)

	// Verify file1.txt is copied
	content, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))

	// Verify todelete.txt is NOT deleted because DeleteDestination is false
	_, err = os.Stat(filepath.Join(dstDir, "todelete.txt"))
	require.NoError(t, err)
}

func TestSync_SyncWithDelete(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	require.NoError(t, os.Mkdir(srcDir, 0755))
	require.NoError(t, os.Mkdir(dstDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "todelete.txt"), []byte("delete me"), 0644))

	s, err := New(context.Background(), WithDelete(true))
	require.NoError(t, err)

	err = s.Sync(context.Background(), srcDir, dstDir)
	require.NoError(t, err)

	// Verify file1.txt is copied
	content, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))

	// Verify todelete.txt is deleted because DeleteDestination is true
	_, err = os.Stat(filepath.Join(dstDir, "todelete.txt"))
	require.True(t, os.IsNotExist(err))
}

func TestSync_WithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "sub"), 0755))
	require.NoError(t, os.MkdirAll(dstDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "keep1.txt"), []byte("keep"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "ignore1.tmp"), []byte("ignore"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub/keep2.txt"), []byte("keep"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub/ignore2.tmp"), []byte("ignore"), 0644))

	f := filter.Opt
	f.MinAge = fs.DurationOff
	f.MaxAge = fs.DurationOff
	f.FilterRule = []string{"- *.tmp"}

	s, err := New(context.Background(), WithFilterOpt(f))
	require.NoError(t, err)

	err = s.Sync(context.Background(), srcDir, dstDir)
	require.NoError(t, err)

	expectedFiles := []string{
		"keep1.txt",
		"sub/keep2.txt",
	}
	for _, file := range expectedFiles {
		_, err := os.Stat(filepath.Join(dstDir, file))
		require.NoError(t, err, "File %s should exist", file)
	}

	ignoredFiles := []string{
		"ignore1.tmp",
		"sub/ignore2.tmp",
	}
	for _, file := range ignoredFiles {
		_, err := os.Stat(filepath.Join(dstDir, file))
		require.True(t, os.IsNotExist(err), "File %s should not exist", file)
	}
}
