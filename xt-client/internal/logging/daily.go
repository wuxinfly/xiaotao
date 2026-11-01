// Package logging routes the client runtime log into dated files inside a logs directory.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	prefix     = "client-"
	suffix     = ".log"
	dateLayout = "2006-01-02"
)

// Daily writes log lines to <dir>/client-YYYY-MM-DD.log, using local time, and
// switches to a new file when the date changes. Files are opened in append mode,
// so restarts and concurrent processes keep earlier lines.
type Daily struct {
	dir string
	now func() time.Time

	mu   sync.Mutex
	file *os.File
	day  string
	path string
}

// Open creates the logs directory if needed and opens the file for the current day.
func Open(dir string) (*Daily, error) {
	if dir == "" {
		return nil, fmt.Errorf("open logs: empty directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("open logs: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create logs directory: %w", err)
	}
	d := &Daily{dir: abs, now: time.Now}
	if err := d.rotate(); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

func (d *Daily) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.rotate(); err != nil {
		return 0, err
	}
	return d.file.Write(p)
}

func (d *Daily) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	f := d.file
	d.file, d.day = nil, ""
	if f == nil {
		return nil
	}
	return f.Close()
}

// Path returns the log file currently receiving writes.
func (d *Daily) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path
}

// Dir returns the logs directory.
func (d *Daily) Dir() string { return d.dir }

// rotate opens today's file when it is missing or the local date changed.
// Callers must hold d.mu.
func (d *Daily) rotate() error {
	day := d.now().Format(dateLayout)
	if d.file != nil && day == d.day {
		return nil
	}
	path := filepath.Join(d.dir, prefix+day+suffix)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	prev := d.file
	d.file, d.day, d.path = f, day, path
	if prev != nil {
		_ = prev.Close()
	}
	return nil
}
