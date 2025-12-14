package syncer

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// WalkLocal lists all files in a local directory.
// It returns a map where the keys are the relative paths from the root.
func WalkLocal(root string, filter func(string) bool) (map[string]ObjectInfo, error) {
	objects := make(map[string]ObjectInfo)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Apply filter if provided
		if filter != nil && !filter(relPath) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info: %w", err)
		}

		objects[relPath] = ObjectInfo{
			Key:     relPath, // Use relative path as key for consistency
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk local directory: %w", err)
	}

	return objects, nil
}
