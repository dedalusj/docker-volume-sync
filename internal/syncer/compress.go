package syncer

import (
	"fmt"
	"strings"
)

const (
	compressMode  = "gzip"
	compressLevel = 5
)

// WrapCompress wraps an rclone remote in the compress backend, returning an
// rclone connection string. The remote is returned unchanged when compression
// is disabled.
func WrapCompress(remote string, enabled bool) string {
	if !enabled {
		return remote
	}

	// rclone quotes connection string values with single quotes and escapes a
	// literal quote by doubling it.
	quoted := strings.ReplaceAll(remote, "'", "''")

	return fmt.Sprintf(":compress,mode=%s,level=%d,remote='%s':", compressMode, compressLevel, quoted)
}
