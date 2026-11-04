package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendLoadsCurrentProjectInstructions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		prompts = append(prompts, body.Messages[0].Content)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{"first rule", "updated rule"} {
		if err := os.WriteFile(path, []byte(rule), 0600); err != nil {
			t.Fatal(err)
		}
		if err := svc.Send(context.Background(), session.ID, "hello", func(Event) {}); err != nil {
			t.Fatal(err)
		}
	}
	if len(prompts) != 2 || !strings.Contains(prompts[0], "first rule") || !strings.Contains(prompts[1], "updated rule") {
		t.Fatalf("unexpected prompts: %v", prompts)
	}
	if !strings.Contains(prompts[0], "以下是完整 XiaoTao SKILL.md") || !strings.Contains(prompts[0], "## 核心约束") {
		t.Fatal("first model request did not receive the full Core")
	}
	if strings.Contains(prompts[1], "以下是完整 XiaoTao SKILL.md") || !strings.Contains(prompts[1], "持续守卫") {
		t.Fatal("subsequent model request did not switch to on-demand Core")
	}
}

func TestProjectInstructionsPriorityAndLimit(t *testing.T) {
	root := t.TempDir()
	w, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(root, "AGENTS.override.md")
	if err := os.WriteFile(override, []byte("override"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := w.projectInstructions()
	if err != nil || !strings.Contains(got, "override") || strings.Contains(got, "base") {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := os.WriteFile(override, []byte("  "), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = w.projectInstructions()
	if err != nil || !strings.Contains(got, "base") {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := os.WriteFile(override, []byte(strings.Repeat("x", maxProjectInstructionsBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.projectInstructions(); err == nil {
		t.Fatal("expected oversized instruction error")
	}
}
