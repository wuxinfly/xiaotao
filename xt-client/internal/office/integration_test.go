package office

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/agent"
)

// TestOfficeWithRealKernel drives the office through the real agent kernel and
// a stub model server, so the adapter, status events and artifact detection
// are exercised together rather than against a fake executor.
func TestOfficeWithRealKernel(t *testing.T) {
	var calls int
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if !bytes.Contains(body, []byte(`"tools"`)) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"确认请求。"}}]}`))
			return
		}
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.md\",\"content\":\"hello\"}"}}]}}]}`))
			return
		}
		if !bytes.Contains(body, []byte(`"tool_call_id":"call_1"`)) {
			t.Errorf("tool result missing: %s", body)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"写好了。"}}]}`))
	}))
	defer modelServer.Close()

	dataDir := t.TempDir()
	agentSvc := agent.NewService(filepath.Join(dataDir, "agent"), modelServer.URL, nil, modelServer.Client(),
		func() []agent.ModelOption { return []agent.ModelOption{{Model: "test/model", Label: "测试模型"}} })
	svc, err := New(filepath.Join(dataDir, "office"), NewAgentExecutor(agentSvc))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	project := t.TempDir()
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, ProjectName: "项目", Goal: "写一个结果文件", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := svc.TaskDetail(project, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if isTerminal(detail.Task.Status) {
			if detail.Task.Status != StatusCompleted {
				t.Fatalf("task failed: %s %s", detail.Task.Status, detail.Task.Error)
			}
			if len(detail.Artifacts) != 1 || detail.Artifacts[0].Path != "out.md" {
				t.Fatalf("artifact: %+v", detail.Artifacts)
			}
			if detail.Task.Stats.ToolCalls < 1 || detail.Task.Stats.FilesWritten < 1 {
				t.Fatalf("stats: %+v", detail.Task.Stats)
			}
			content, err := os.ReadFile(filepath.Join(project, "out.md"))
			if err != nil || string(content) != "hello" {
				t.Fatalf("file: %q %v", content, err)
			}
			raw, _ := json.Marshal(detail.History)
			if !strings.Contains(string(raw), "写好了") {
				t.Fatalf("transcript missing final answer: %s", raw)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("task did not finish")
}
