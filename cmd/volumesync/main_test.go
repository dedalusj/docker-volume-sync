package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChownDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	
	subDir1 := filepath.Join(tmpDir, "sub1")
	subDir2 := filepath.Join(tmpDir, "sub1", "sub2")
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(subDir1, "file2.txt")
	
	require.NoError(t, os.Mkdir(subDir1, 0755))
	require.NoError(t, os.Mkdir(subDir2, 0755))
	require.NoError(t, os.WriteFile(file1, []byte("test"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("test"), 0644))
	
	// We can't easily change UID/GID to anything other than ourselves without root
	// But we can test if it doesn't fail when we pass our own UID/GID
	uid := os.Getuid()
	gid := os.Getgid()
	
	err := chownDirectories(tmpDir, uid, gid)
	assert.NoError(t, err)
	
	// Test with -1 (no change)
	err = chownDirectories(tmpDir, -1, -1)
	assert.NoError(t, err)
}
