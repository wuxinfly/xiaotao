package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendWithReferencesReturnsReferenceToModelAndTool(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		if len(payload.Tools) == 0 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"确认引用请求。"}}]}`))
			return
		}
		raw, _ := json.Marshal(payload)
		requests = append(requests, string(raw))
		if len(requests) == 1 {
			toolFound := false
			for _, tool := range payload.Tools {
				if tool["function"].(map[string]any)["name"] == "read_conversation_reference" {
					toolFound = true
				}
			}
			if !toolFound {
				t.Error("reference tool missing")
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_ref","type":"function","function":{"name":"list_files","arguments":"{}"}}]}}]}`))
			return
		}
		if !strings.Contains(requests[len(requests)-1], `"tool_call_id":"call_ref"`) {
			t.Error("tool result missing")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"引用了旧结论。"}}]}`))
	}))
	defer server.Close()

	svc := NewService(t.TempDir(), server.URL, func() string { return "key" }, server.Client(), nil)
	project := t.TempDir()
	source, err := svc.Create(Options{ProjectPath: project, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	source.Title = "旧讨论"
	source.Messages = []Message{{ID: "m", Role: "user", Content: "决定用 A 方案", CreatedAt: time.Now()}}
	if err := svc.saveSession(source); err != nil {
		t.Fatal(err)
	}
	target, err := svc.Create(Options{ProjectPath: project, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.SendWithReferences(context.Background(), target.ID, "接着 A 做", "", []string{source.ID}, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests: %d", len(requests))
	}
	if !strings.Contains(requests[0], "决定用 A 方案") || !strings.Contains(requests[0], "接着 A 做") {
		t.Fatalf("model did not receive frozen summary: %s", requests[0])
	}
	loaded, err := svc.Get(target.ID)
	if err != nil || len(loaded.Messages) != 4 || len(loaded.Messages[0].References) != 1 || !loaded.Messages[0].References[0].SourceAvailable {
		t.Fatalf("reference message not persisted: %+v %v", loaded, err)
	}
	if len(loaded.Activity) != 1 || len(loaded.RunStats) != 1 || loaded.RunStats[0].ModelRequests != 2 {
		t.Fatalf("missing run timeline: %+v", loaded)
	}
}
