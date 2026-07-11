package syncer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rclone/rclone/fs/fspath"
	"github.com/stretchr/testify/require"
)

func TestWrapCompress(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		enabled bool
		want    string
	}{
		{
			name:    "Disabled returns remote unchanged",
			remote:  "s3:my-bucket/db_data",
			enabled: false,
			want:    "s3:my-bucket/db_data",
		},
		{
			name:    "Enabled wraps in compress backend",
			remote:  "s3:my-bucket/db_data",
			enabled: true,
			want:    ":compress,mode=gzip,level=5,remote='s3:my-bucket/db_data':",
		},
		{
			name:    "Single quotes in remote are doubled",
			remote:  "s3:my-bucket/it's_data",
			enabled: true,
			want:    ":compress,mode=gzip,level=5,remote='s3:my-bucket/it''s_data':",
		},
		{
			name:    "Local path",
			remote:  "/volumes/db_data",
			enabled: true,
			want:    ":compress,mode=gzip,level=5,remote='/volumes/db_data':",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapCompress(tt.remote, tt.enabled)
			require.Equal(t, tt.want, got)

			// A malformed connection string must not silently pass: make sure
			// rclone itself can parse what we produced.
			if tt.enabled {
				_, err := fspath.Parse(got)
				require.NoError(t, err, "rclone should be able to parse the connection string")
			}
		})
	}
}

// TestSync_CompressRoundTrip is the real proof: sync into a compressed remote,
// verify the on-disk layout is compressed, then sync back out and verify the
// files come back byte-identical.
func TestSync_CompressRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	restoreDir := filepath.Join(tmpDir, "restore")

	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "sub"), 0755))
	require.NoError(t, os.Mkdir(dstDir, 0755))
	require.NoError(t, os.Mkdir(restoreDir, 0755))

	files := map[string]string{
		"file1.txt":     "hello world",
		"sub/file2.txt": strings.Repeat("compress me ", 100),
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0644))
	}

	s, err := New(context.Background())
	require.NoError(t, err)

	// Backup: local -> compressed remote.
	compressedDst := WrapCompress(dstDir, true)
	require.NoError(t, s.Sync(context.Background(), srcDir, compressedDst))

	// The destination must hold compress-backend artefacts, not plaintext copies.
	// Every file gets a .json metadata sidecar, but the data file is only .gz
	// when gzip actually shrinks it — the backend falls back to storing the
	// bytes as .bin otherwise (a few bytes of text gzip to more than they
	// started with). So the sidecar is the invariant, not the .gz.
	var gzCount, binCount, jsonCount int
	require.NoError(t, filepath.Walk(dstDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch filepath.Ext(p) {
		case ".gz":
			gzCount++
		case ".bin":
			binCount++
		case ".json":
			jsonCount++
		}
		return nil
	}))
	require.Equal(t, len(files), jsonCount, "each file should have a metadata sidecar")
	require.Equal(t, len(files), gzCount+binCount, "each file should have a data file")
	require.NotZero(t, gzCount, "the compressible file should be stored gzipped")

	for name := range files {
		_, err := os.Stat(filepath.Join(dstDir, name))
		require.True(t, os.IsNotExist(err), "%s should not be stored as plaintext", name)
	}

	// Restore: compressed remote -> local.
	require.NoError(t, s.Sync(context.Background(), compressedDst, restoreDir))

	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(restoreDir, name))
		require.NoError(t, err, "%s should be restored", name)
		require.Equal(t, content, string(got), "%s should round trip unchanged", name)
	}
}

// TestSync_CompressActuallyShrinks guards against compression silently
// no-op'ing, which the round-trip test alone would not catch.
func TestSync_CompressActuallyShrinks(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	require.NoError(t, os.Mkdir(srcDir, 0755))
	require.NoError(t, os.Mkdir(dstDir, 0755))

	// Highly compressible: ~1MB of repeated text.
	content := strings.Repeat("the quick brown fox jumps over the lazy dog\n", 25000)
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "big.txt"), []byte(content), 0644))

	s, err := New(context.Background())
	require.NoError(t, err)

	require.NoError(t, s.Sync(context.Background(), srcDir, WrapCompress(dstDir, true)))

	var dstSize int64
	require.NoError(t, filepath.Walk(dstDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			dstSize += info.Size()
		}
		return nil
	}))

	srcSize := int64(len(content))
	require.Less(t, dstSize, srcSize/10,
		"compressed destination (%d bytes) should be far smaller than the source (%d bytes)", dstSize, srcSize)
}
