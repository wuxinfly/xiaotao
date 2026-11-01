package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSplitsFilesByDay(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	day := time.Date(2026, 10, 31, 23, 59, 0, 0, time.Local)
	d.now = func() time.Time { return day }
	if _, err := d.Write([]byte("yesterday\n")); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, "client-2026-10-31.log")
	if got := d.Path(); got != first {
		t.Fatalf("path = %q, want %q", got, first)
	}

	day = day.Add(time.Minute)
	if _, err := d.Write([]byte("today\n")); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "client-2026-11-01.log")
	if got := d.Path(); got != second {
		t.Fatalf("path after rollover = %q, want %q", got, second)
	}

	if b, err := os.ReadFile(first); err != nil || string(b) != "yesterday\n" {
		t.Fatalf("previous day file = %q, err %v", b, err)
	}
	if b, err := os.ReadFile(second); err != nil || string(b) != "today\n" {
		t.Fatalf("current day file = %q, err %v", b, err)
	}
}

func TestReopenAppendsSameDay(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	path := filepath.Join(dir, "client-"+time.Now().Format("2006-01-02")+".log")

	for _, line := range []string{"first\n", "second\n"} {
		d, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := d.Path(); got != path {
			t.Fatalf("path = %q, want %q", got, path)
		}
		if _, err := d.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "first\nsecond\n" {
		t.Fatalf("log content = %q", b)
	}
}

func TestOpenRejectsEmptyDir(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("expected error for empty directory")
	}
}
