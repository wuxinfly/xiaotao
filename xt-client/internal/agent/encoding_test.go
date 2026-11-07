package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The stage-save contract pipes JSON into a python script through run_command.
// Chinese Windows previously replaced every non-ASCII byte on that pipe with "?",
// so a saved stage lost its title and summary. Keep the round trip byte-exact.
func TestRunCommandPipesChineseToChild(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows console encoding regression")
	}
	root := t.TempDir()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(root, "payload.json")
	if err := os.WriteFile(payload, []byte(`{"summary":"中文摘要","n":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "echo.json")
	command := `Get-Content -LiteralPath '` + payload + `' -Raw -Encoding UTF8 | python -c "import sys;open(r'` + out + `','wb').write(sys.stdin.buffer.read())"`
	if _, err := workspace.Execute(context.Background(), "run_command", mustJSON(t, map[string]any{"command": command})); err != nil {
		t.Fatalf("run_command: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "?") || !strings.Contains(string(raw), "中文摘要") {
		t.Fatalf("chinese lost on the stdin pipe: %q", raw)
	}
}

// PowerShell's own output is redirected into the tool result, so cmdlet output
// must be UTF-8 instead of the console code page.
func TestRunCommandReturnsChineseStdout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows console encoding regression")
	}
	workspace, err := OpenWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Execute(context.Background(), "run_command", mustJSON(t, map[string]any{"command": `Write-Output '中文输出'`}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "中文输出") {
		t.Fatalf("cmdlet output was not UTF-8: %q", output)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
