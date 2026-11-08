package logging

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    Level
		wantErr bool
	}{
		{"debug", LevelDebug, false},
		{"DEBUG", LevelDebug, false},
		{"Info", LevelInfo, false},
		{"warn", LevelWarn, false},
		{"warning", LevelWarn, false},
		{"ERROR", LevelError, false},
		{"invalid", LevelInfo, true},
		{"", LevelInfo, true},
	}
	for _, tc := range tests {
		got, err := ParseLevel(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseLevel(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestModulesAndDirs(t *testing.T) {
	tmp := t.TempDir()
	set, err := Open(tmp, LevelInfo)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer set.Close()

	for _, m := range allModules {
		logger := set.For(m)
		logger.Info("hello " + string(m))
		expectedFile := filepath.Join(tmp, string(m), string(m)+"-"+time.Now().Format("2006-01-02")+".log")
		if logger.Path() != expectedFile {
			t.Errorf("module %s: expected path %s, got %s", m, expectedFile, logger.Path())
		}
		data, err := os.ReadFile(expectedFile)
		if err != nil || !strings.Contains(string(data), "hello "+string(m)) {
			t.Errorf("module %s log content missing or unreadable: %v", m, err)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	tmp := t.TempDir()
	set, err := Open(tmp, LevelInfo)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer set.Close()

	l := set.For(ModuleClient)
	l.Debug("should not appear")
	l.Info("should appear")

	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(data), "should not appear") {
		t.Error("debug line logged despite LevelInfo threshold")
	}
	if !strings.Contains(string(data), "should appear") {
		t.Error("info line missing")
	}

	tmpDebug := t.TempDir()
	setDebug, err := Open(tmpDebug, LevelDebug)
	if err != nil {
		t.Fatalf("Open debug failed: %v", err)
	}
	defer setDebug.Close()

	lDebug := setDebug.For(ModuleClient)
	lDebug.Debug("debug appeared")
	dataDebug, _ := os.ReadFile(lDebug.Path())
	if !strings.Contains(string(dataDebug), "debug appeared") {
		t.Error("debug line missing in LevelDebug")
	}
}

func TestRotateAcrossDays(t *testing.T) {
	tmp := t.TempDir()
	set, err := Open(tmp, LevelInfo)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer set.Close()

	l := set.For(ModuleClient)
	day1 := time.Date(2026, 11, 8, 10, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 11, 9, 10, 0, 0, 0, time.Local)

	l.now = func() time.Time { return day1 }
	l.Info("day 1 entry")
	p1 := l.Path()

	l.now = func() time.Time { return day2 }
	l.Info("day 2 entry")
	p2 := l.Path()

	if p1 == p2 {
		t.Errorf("expected different paths across days, got %s", p1)
	}
	d1, _ := os.ReadFile(p1)
	d2, _ := os.ReadFile(p2)
	if !strings.Contains(string(d1), "day 1 entry") {
		t.Errorf("day 1 content missing in %s", p1)
	}
	if !strings.Contains(string(d2), "day 2 entry") {
		t.Errorf("day 2 content missing in %s", p2)
	}
}

func TestFormatFields(t *testing.T) {
	fields := []Field{
		F("status", 200),
		F("msg", "hello world"),
		F("tag", "alpha"),
		F("tag", "beta"),
		F("complex", "a=b"),
	}
	out := formatFields(fields)
	want := `status=200 msg="hello world" tag=alpha,beta complex="a=b"`
	if out != want {
		t.Errorf("formatFields = %q, want %q", out, want)
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"authorization: Bearer 12345", 300, "authorization=<redacted>"},
		{"failed to login with token=abcde", 300, "failed to login with token=<redacted>"},
		{"password is secret123", 300, "password=<redacted>"},
		{"normal error message", 10, "normal err..."},
		{"normal error message", 0, "normal error message"},
	}
	for _, tc := range tests {
		got := Redact(tc.input, tc.maxLen)
		if got != tc.want {
			t.Errorf("Redact(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
		}
	}
}

func TestCleanupAndArchive(t *testing.T) {
	tmp := t.TempDir()
	flatOld := filepath.Join(tmp, "client-2026-10-01.log")
	_ = os.WriteFile(flatOld, []byte("old flat"), 0o600)

	modDir := filepath.Join(tmp, "proxy")
	_ = os.MkdirAll(modDir, 0o700)
	modOld := filepath.Join(modDir, "proxy-2026-10-01.log")
	_ = os.WriteFile(modOld, []byte("old mod"), 0o600)
	modRecent := filepath.Join(modDir, "proxy-2026-11-08.log")
	_ = os.WriteFile(modRecent, []byte("recent mod"), 0o600)

	removed, archived, err := Cleanup(tmp, 14)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if archived != 1 {
		t.Errorf("archived = %d, want 1", archived)
	}
	if removed < 1 {
		t.Errorf("removed = %d, want >= 1", removed)
	}
	if _, err := os.Stat(modRecent); err != nil {
		t.Errorf("recent mod log was deleted: %v", err)
	}
}

func TestContextReqID(t *testing.T) {
	ctx := context.Background()
	if got := ReqID(ctx); got != "" {
		t.Errorf("expected empty req id, got %q", got)
	}
	ctx = WithReqID(ctx, "req_12345678")
	if got := ReqID(ctx); got != "req_12345678" {
		t.Errorf("expected req_12345678, got %q", got)
	}
}

func TestDefaultNilSafety(t *testing.T) {
	SetDefault(nil)
	logger := Default().For(ModuleProxy)
	if logger == nil {
		t.Fatal("expected non-nil discard logger")
	}
	logger.Info("should not panic")
	if p := logger.Path(); p != "" {
		t.Errorf("expected empty path from discard logger, got %q", p)
	}
}

func TestOpenAppendMode(t *testing.T) {
	tmp := t.TempDir()
	s1, err := Open(tmp, LevelInfo)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	s1.For(ModuleClient).Info("first line")
	p := s1.For(ModuleClient).Path()
	_ = s1.Close()

	s2, err := Open(tmp, LevelInfo)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	s2.For(ModuleClient).Info("second line")
	_ = s2.Close()

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "first line") || !strings.Contains(content, "second line") {
		t.Errorf("expected both lines appended, got: %s", content)
	}
}
