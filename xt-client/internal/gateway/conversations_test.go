package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardSeparatesExternalResponsesThreads(t *testing.T) {
	svc, src, usage := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("thread-id") != "" {
			t.Error("incoming session metadata leaked to upstream")
		}
		completion(w)
	})
	for turn := 0; turn < 3; turn++ {
		for _, thread := range []string{"first", "second"} {
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":%q,"input":"turn %d"}`, src.Models[0].ID, turn)))
			req.Header.Set("Authorization", "Bearer local-key")
			req.Header.Set("thread-id", thread)
			req.Header.Set("session-id", "shared-client")
			out := httptest.NewRecorder()
			svc.HandleResponses(out, req)
			if out.Code != 200 {
				t.Fatalf("response status %d: %s", out.Code, out.Body.String())
			}
		}
	}
	snap, err := usage.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Conversations) != 2 {
		t.Fatalf("conversations: %+v", snap.Conversations)
	}
	for _, item := range snap.Conversations {
		if item.Active != 0 || len(item.Requests) != 3 || item.Context.Window != 4096 || item.Context.Estimated <= 0 || item.Totals.Prompt == nil || *item.Totals.Prompt != 60 {
			t.Fatalf("unexpected conversation: %+v", item)
		}
	}
}

func TestDashboardDoesNotMergeAmbiguousHistory(t *testing.T) {
	svc, src, usage := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { completion(w) })
	histories := []string{
		`[{"role":"user","content":"same"}]`,
		`[{"role":"user","content":"same"}]`,
		`[{"role":"user","content":"same"},{"role":"assistant","content":"ok"},{"role":"user","content":"more"}]`,
	}
	for _, history := range histories {
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":%s}`, src.Models[0].ID, history)))
		req.Header.Set("Authorization", "Bearer local-key")
		out := httptest.NewRecorder()
		svc.HandleChat(out, req)
		if out.Code != 200 {
			t.Fatalf("response status %d: %s", out.Code, out.Body.String())
		}
	}
	snap, err := usage.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Conversations) != 3 {
		t.Fatalf("ambiguous history merged: %+v", snap.Conversations)
	}
}

func TestDashboardGroupsExplicitAPIAndAnthropicSessions(t *testing.T) {
	svc, src, usage := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { completion(w) })
	for i := 0; i < 2; i++ {
		external := chatRequest(src.Models[0].ID, false)
		external.Header.Set("X-Xiaotao-Session-Id", "same-id")
		responses := httptest.NewRecorder()
		svc.HandleChat(responses, external)
		if responses.Code != 200 {
			t.Fatalf("Chat status %d: %s", responses.Code, responses.Body.String())
		}
		body := fmt.Sprintf(`{"model":%q,"max_tokens":32,"messages":[{"role":"user","content":"hello"}],"metadata":{"user_id":"{\"session_id\":\"same-id\"}"}}`, src.Models[0].ID)
		messages := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		messages.Header.Set("x-api-key", "local-key")
		response := httptest.NewRecorder()
		svc.HandleMessages(response, messages)
		if response.Code != 200 {
			t.Fatalf("Messages status %d: %s", response.Code, response.Body.String())
		}
	}
	snap, err := usage.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Conversations) != 2 {
		t.Fatalf("conversations: %+v", snap.Conversations)
	}
	for _, session := range snap.Conversations {
		if len(session.Requests) != 2 {
			t.Fatalf("incorrectly grouped: %+v", session)
		}
	}
}

func TestCodexDashboardKeepsTitleHelpersInMainThread(t *testing.T) {
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { completion(w) })
	svc.codexTitles.path = filepath.Join(t.TempDir(), "session_index.jsonl")
	ask := func(header, id, extra, text string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":%q,"input":%q%s}`, src.Models[0].ID, text, extra)))
		req.Header.Set("Authorization", "Bearer local-key")
		req.Header.Set("originator", "codex_desktop")
		if header != "" {
			req.Header.Set(header, id)
		}
		out := httptest.NewRecorder()
		svc.HandleResponses(out, req)
		if out.Code != 200 {
			t.Fatalf("gateway status %d: %s", out.Code, out.Body.String())
		}
	}
	ask("session_id", "main", "", "检查 Windows 打包")
	ask("thread-id", "main", "", "检查 Windows 打包")
	ask("", "", `,"prompt_cache_key":"main"`, "检查 Windows 打包")
	ask("session_id", "other", "", "另一个对话")
	for range 4 {
		ask("session-id", "helper", `,"client_metadata":{"x-codex-turn-metadata":"{\"thread_id\":\"helper\",\"thread_source\":\"thread_title\",\"parent_thread_id\":\"main\"}"}`, "title")
	}
	if err := os.WriteFile(svc.codexTitles.path, []byte("{\"id\":\"main\",\"thread_name\":\"修复 Windows 打包\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Conversations) != 2 {
		t.Fatalf("wrong groups: %+v", snap.Conversations)
	}
	got := snap.Conversations[0]
	if got.Title != "修复 Windows 打包" || got.Context.Estimated <= 2 || len(got.Context.Items["messages"]) != 1 || got.Totals.Prompt == nil || *got.Totals.Prompt != 140 {
		t.Fatalf("main context or title overwritten: %+v", got)
	}
}
