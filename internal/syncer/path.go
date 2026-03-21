package syncer

import (
	"path/filepath"
)

func JoinPath(base, sub string) string {
	return filepath.Join(base, sub)
}
