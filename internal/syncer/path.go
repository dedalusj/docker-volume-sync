package syncer

import (
	"path/filepath"
	"strings"
)

// JoinPath joined a base path and a sub path, handling rclone remote syntax and s3:// prefixes.
func JoinPath(base, sub string) string {
	// If it's a URL-style remote, normalize it to rclone-style (s3://bucket -> s3:bucket)
	if strings.Contains(base, "://") {
		base = strings.Replace(base, "://", ":", 1)
	}

	// If it's an rclone remote (contains a colon)
	if strings.Contains(base, ":") {
		// Ensure we don't have double slashes but preserve the single slash after colon if it's there
		// BUT rclone prefers bucket/path without leading slash for S3.
		// If base ends with colon, just append sub.
		if strings.HasSuffix(base, ":") {
			return base + strings.TrimPrefix(sub, "/")
		}
		return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(sub, "/")
	}

	// Otherwise, it's a local path
	return filepath.Join(base, sub)
}
