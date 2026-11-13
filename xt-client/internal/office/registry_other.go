//go:build !windows

package office

import "os"

func replaceFile(source, target string) error { return os.Rename(source, target) }

func syncDirectory(path string) {
	if directory, err := os.Open(path); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
}
