package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/logging"
)

func TestAgentLogging_ToolCallNoArgumentsOrFullOutput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "xt-agent-logging-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	set, err := logging.Open(tmpDir, logging.LevelDebug)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer set.Close()

	oldDefault := logging.Default()
	logging.SetDefault(set)
	defer logging.SetDefault(oldDefault)

	secretArg := "super_secret_file_content_or_param"
	secretOutput := "sensitive_output_that_must_not_appear_in_log"

	// Call logToolCall - note it only takes toolName, status, durationMs, outputBytes
	logToolCall("sess-123", "run-456", "read_file", "ok", 42, len(secretOutput))

	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(tmpDir, "agent", "agent-"+today+".log")

	contentBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read agent log: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, `tool=read_file`) {
		t.Fatalf("expected tool=read_file in log, got: %s", content)
	}
	if !strings.Contains(content, `session=sess-123`) {
		t.Fatalf("expected session=sess-123 in log, got: %s", content)
	}
	if !strings.Contains(content, `run=run-456`) {
		t.Fatalf("expected run=run-456 in log, got: %s", content)
	}
	if strings.Contains(content, secretArg) {
		t.Fatalf("log contains secret argument: %s", content)
	}
	if strings.Contains(content, secretOutput) {
		t.Fatalf("log contains raw tool output content: %s", content)
	}
}
