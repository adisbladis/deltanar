package gcroots

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/adisbladis/deltanar/store"
)

func ReadDirectory(gcRootDir string, host string) ([]string, error) {
	path := filepath.Join(gcRootDir, host)

	// TODO: Recursive walkDir(?)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var roots []string
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())

		info, err := os.Lstat(entryPath)
		if err != nil {
			return nil, err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			return nil, fmt.Errorf("path %s is not a symlink", entryPath)
		}

		target, err := os.Readlink(entryPath)
		if err != nil {
			return nil, err
		}

		roots = append(roots, target)
	}

	return store.QueryRequisites(roots...)
}
