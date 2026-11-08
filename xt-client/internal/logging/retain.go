package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cleanup removes log files in module subdirectories and archive/ whose date is older
// than keepDays. It also moves flat *.log files in dir to dir/archive/.
// Returns the number of removed and archived files. If keepDays <= 0, no files are removed.
func Cleanup(dir string, keepDays int) (removed int, archived int, err error) {
	if dir == "" {
		return 0, 0, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, 0, fmt.Errorf("cleanup logs: %w", err)
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	archiveDir := filepath.Join(abs, "archive")

	// 1. Move flat *.log in logs root to archive/
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		if err := os.MkdirAll(archiveDir, 0o700); err != nil {
			return removed, archived, fmt.Errorf("create archive dir: %w", err)
		}
		src := filepath.Join(abs, entry.Name())
		dst := filepath.Join(archiveDir, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			if removeErr := os.Remove(dst); removeErr == nil {
				_ = os.Rename(src, dst)
			}
		}
		archived++
	}

	if keepDays <= 0 {
		return removed, archived, nil
	}

	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -keepDays)

	// 2. Scan subdirectories (modules + archive) and delete expired logs
	subEntries, err := os.ReadDir(abs)
	if err != nil {
		return removed, archived, err
	}

	for _, sub := range subEntries {
		if !sub.IsDir() {
			continue
		}
		subPath := filepath.Join(abs, sub.Name())
		files, err := os.ReadDir(subPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".log") {
				continue
			}
			filePath := filepath.Join(subPath, f.Name())
			fileDate, ok := parseLogDate(f.Name())
			if !ok {
				if info, err := f.Info(); err == nil {
					fileDate = info.ModTime()
				} else {
					continue
				}
			}
			if fileDate.Before(cutoff) {
				if err := os.Remove(filePath); err == nil {
					removed++
				}
			}
		}
	}

	return removed, archived, nil
}

func parseLogDate(name string) (time.Time, bool) {
	base := strings.TrimSuffix(name, ".log")
	if len(base) < 10 {
		return time.Time{}, false
	}
	dateStr := base[len(base)-10:]
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
