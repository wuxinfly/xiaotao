package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/office"
)

type officeFakeExecutor struct {
	project string
}

func (f *officeFakeExecutor) CreateSession(options agent.Options) (*agent.Session, error) {
	return &agent.Session{ID: "run_office", Project: agent.Project{Path: options.ProjectPath, Name: options.ProjectName}, Role: options.Role, Model: options.Model}, nil
}

func (f *officeFakeExecutor) RunTurn(ctx context.Context, sessionID, text, model string, _ *agent.ModelSettings, emit func(agent.Event)) error {
	emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_1"})
	emit(agent.Event{Type: "activity", SessionID: sessionID, Activity: &agent.Activity{ID: "act_1", RunID: "turn_1", Kind: "write_file", Title: "更新文件", Input: `{"path":"result.md"}`, Status: "completed", StartedAt: time.Now()}})
	emit(agent.Event{Type: "status", SessionID: sessionID, RunID: "turn_1", Status: agent.StatusCompleted})
	return nil
}

func (f *officeFakeExecutor) Cancel(string) bool { return true }

func (f *officeFakeExecutor) History(sessionID, _ string) (*agent.HistoryPage, error) {
	return &agent.HistoryPage{Session: &agent.Session{ID: sessionID, Messages: []agent.Message{}, Activity: []agent.Activity{}}}, nil
}

func (f *officeFakeExecutor) ResolveApproval(string, string, bool) error { return nil }

func (f *officeFakeExecutor) ListSummaries() ([]*agent.Session, error) { return nil, nil }

func (f *officeFakeExecutor) AvailableModels() []agent.ModelOption {
	return []agent.ModelOption{{Model: "test/model", Label: "测试模型"}}
}

func (f *officeFakeExecutor) AccessMode() string { return agent.ModeSafe }

func newOfficeTestMux(t *testing.T) (*http.ServeMux, *office.Service) {
	t.Helper()
	exec := &officeFakeExecutor{}
	svc, err := office.New(t.TempDir(), exec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	mux := http.NewServeMux()
	registerOfficeRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("X-Requested-With") == "XiaoTao" },
		func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) },
		func(w http.ResponseWriter, code int, msg string) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
		})
	return mux, svc
}

func TestOfficeRoutesRequireSameOrigin(t *testing.T) {
	mux, _ := newOfficeTestMux(t)
	for _, target := range []struct{ method, path string }{
		{"GET", "/api/office/snapshot?projectPath=x"},
		{"POST", "/api/office/tasks"},
		{"GET", "/api/office/tasks/task_1?projectPath=x"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(target.method, target.path, strings.NewReader("{}")))
		if w.Code != 403 {
			t.Fatalf("%s %s without origin: %d", target.method, target.path, w.Code)
		}
	}
}

func TestOfficeTaskFlowOverHTTP(t *testing.T) {
	mux, _ := newOfficeTestMux(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "result.md"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(office.CreateTaskRequest{ProjectPath: project, ProjectName: "项目", Goal: "写个结果", Model: "test/model"})
	req := httptest.NewRequest("POST", "/api/office/tasks", strings.NewReader(string(body)))
	req.Header.Set("X-Requested-With", "XiaoTao")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create task: %d %s", w.Code, w.Body.String())
	}
	var task office.Task
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		detailReq := httptest.NewRequest("GET", "/api/office/tasks/"+task.ID+"?projectPath="+strings.ReplaceAll(project, `\`, `%5C`), nil)
		detailReq.Header.Set("X-Requested-With", "XiaoTao")
		detail := httptest.NewRecorder()
		mux.ServeHTTP(detail, detailReq)
		if detail.Code != 200 {
			t.Fatalf("detail: %d %s", detail.Code, detail.Body.String())
		}
		var page office.TaskDetail
		if err := json.Unmarshal(detail.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Task.Status == office.StatusCompleted {
			if len(page.Artifacts) != 1 || page.Artifacts[0].Path != "result.md" {
				t.Fatalf("artifact missing: %+v", page.Artifacts)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("task did not complete")
}

func TestOfficeUnknownTaskIs404(t *testing.T) {
	mux, _ := newOfficeTestMux(t)
	req := httptest.NewRequest("GET", "/api/office/tasks/missing?projectPath="+strings.ReplaceAll(t.TempDir(), `\`, `%5C`), nil)
	req.Header.Set("X-Requested-With", "XiaoTao")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("unknown task: %d %s", w.Code, w.Body.String())
	}
}

func TestOfficeSessionsStayOutOfAgentList(t *testing.T) {
	svc := agent.NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	visible, err := svc.Create(agent.Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Create(agent.Options{Model: "test", Role: "executor"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAgentRoutes(mux, svc, func(*http.Request) bool { return true },
		func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) },
		func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/agent/sessions?summary=1", nil))
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Sessions []agent.Session `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].ID != visible.ID {
		t.Fatalf("executor session leaked into agent list: %+v", body.Sessions)
	}
}
