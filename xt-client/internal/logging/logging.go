package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level: %q", s)
	}
}

type Module string

const (
	ModuleClient Module = "client"
	ModuleProxy  Module = "proxy"
	ModuleAgent  Module = "agent"
	ModuleAuth   Module = "auth"
	ModuleUsage  Module = "usage"
)

var allModules = []Module{
	ModuleClient,
	ModuleProxy,
	ModuleAgent,
	ModuleAuth,
	ModuleUsage,
}

func validModule(m Module) bool {
	switch m {
	case ModuleClient, ModuleProxy, ModuleAgent, ModuleAuth, ModuleUsage:
		return true
	default:
		return false
	}
}

var (
	defaultMu     sync.RWMutex
	defaultSet    *Set
	discardLogger = &Logger{discard: true}
)

func SetDefault(s *Set) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultSet = s
}

func Default() *Set {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultSet
}

type Set struct {
	dir     string
	level   Level
	now     func() time.Time
	mu      sync.Mutex
	loggers map[Module]*Logger
}

func Open(dir string, level Level) (*Set, error) {
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
	s := &Set{
		dir:     abs,
		level:   level,
		now:     time.Now,
		loggers: make(map[Module]*Logger, len(allModules)),
	}
	for _, m := range allModules {
		subDir := filepath.Join(abs, string(m))
		l := &Logger{
			module: m,
			dir:    subDir,
			level:  level,
			now:    s.now,
		}
		if err := l.rotate(); err != nil {
			_ = s.Close()
			return nil, err
		}
		s.loggers[m] = l
	}
	return s, nil
}

func (s *Set) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for _, l := range s.loggers {
		if err := l.closeFile(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Set) For(m Module) *Logger {
	if s == nil {
		return discardLogger
	}
	if !validModule(m) {
		m = ModuleClient
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.loggers[m]; ok {
		return l
	}
	return discardLogger
}

func (s *Set) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

type Logger struct {
	module  Module
	dir     string
	level   Level
	now     func() time.Time
	discard bool

	mu   sync.Mutex
	file *os.File
	day  string
	path string
}

func (l *Logger) Debug(msg string, fields ...Field) { l.Log(LevelDebug, msg, fields...) }
func (l *Logger) Info(msg string, fields ...Field)  { l.Log(LevelInfo, msg, fields...) }
func (l *Logger) Warn(msg string, fields ...Field)  { l.Log(LevelWarn, msg, fields...) }
func (l *Logger) Error(msg string, fields ...Field) { l.Log(LevelError, msg, fields...) }

func (l *Logger) Log(level Level, msg string, fields ...Field) {
	if l == nil || l.discard || level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.rotate(); err != nil {
		return
	}
	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	line := formatLine(now, level, l.module, fields, msg)
	_, _ = l.file.WriteString(line)
}

func (l *Logger) Path() string {
	if l == nil || l.discard {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

func (l *Logger) Write(p []byte) (int, error) {
	if l == nil || l.discard {
		return len(p), nil
	}
	msg := strings.TrimRight(string(p), "\r\n")
	l.Log(LevelInfo, msg)
	return len(p), nil
}

var _ io.Writer = (*Logger)(nil)

func (l *Logger) closeFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.file
	l.file, l.day = nil, ""
	if f == nil {
		return nil
	}
	return f.Close()
}

func (l *Logger) rotate() error {
	day := time.Now().Format("2006-01-02")
	if l.now != nil {
		day = l.now().Format("2006-01-02")
	}
	if l.file != nil && day == l.day {
		return nil
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return fmt.Errorf("create log module dir %s: %w", l.dir, err)
	}
	path := filepath.Join(l.dir, fmt.Sprintf("%s-%s.log", l.module, day))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	prev := l.file
	l.file, l.day, l.path = f, day, path
	if prev != nil {
		_ = prev.Close()
	}
	return nil
}

func formatLine(t time.Time, level Level, module Module, fields []Field, msg string) string {
	var b strings.Builder
	b.WriteString(t.Format("2006/01/02 15:04:05.000"))
	b.WriteByte(' ')
	b.WriteString(level.String())
	b.WriteByte(' ')
	b.WriteString(string(module))

	formattedFields := formatFields(fields)
	if formattedFields != "" {
		b.WriteByte(' ')
		b.WriteString(formattedFields)
	}

	if msg != "" {
		cleanMsg := strings.ReplaceAll(msg, "\r\n", "\\n")
		cleanMsg = strings.ReplaceAll(cleanMsg, "\n", "\\n")
		cleanMsg = strings.ReplaceAll(cleanMsg, "\r", "\\n")
		b.WriteByte(' ')
		b.WriteString(cleanMsg)
	}
	b.WriteByte('\n')
	return b.String()
}
