package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPFlowWithMockModel(t *testing.T) {
	var calls atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("unexpected model request")
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			if !bytes.Contains(body, []byte(`"tools"`)) {
				t.Errorf("tool schema missing")
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_files","arguments":"{\"path\":\".\"}"}}]}}]}`))
			return
		}
		if !bytes.Contains(body, []byte(`"tool_call_id":"call_1"`)) {
			t.Errorf("tool result not returned to model: %s", body)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"项目结构已看过。"}}]}`))
	}))
	defer modelServer.Close()

	data := t.TempDir()
	project := t.TempDir()
	svc := NewService(data, modelServer.URL, func() string { return "user-token" }, modelServer.Client(), nil)
	session, err := svc.Create(Options{ProjectPath: project, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	var events strings.Builder
	err = svc.Send(context.Background(), session.ID, "看看项目结构", func(event Event) {
		if event.Message != nil {
			events.WriteString(event.Message.Content)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two model calls, got %d", calls.Load())
	}
	if !strings.Contains(events.String(), "项目结构已看过。") {
		t.Fatalf("missing final agent message: %s", events.String())
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusCompleted || len(loaded.Activity) != 1 || loaded.Activity[0].Status != "completed" {
		t.Fatalf("unexpected final session: %+v", loaded)
	}
	raw, _ := json.Marshal(loaded)
	if !bytes.Contains(raw, []byte(`"role":"tool"`)) {
		t.Fatalf("tool result was not persisted: %s", raw)
	}
}

func TestSwitchModelBetweenMessages(t *testing.T) {
	var models []string
	var histories [][]map[string]any
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		models = append(models, payload.Model)
		histories = append(histories, payload.Messages)
		if len(models) == 2 {
			close(started)
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"reply"}}]}`))
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, func() string { return "key" }, server.Client(), nil)
	session, err := svc.Create(Options{Model: "first"})
	if err != nil {
		t.Fatal(err)
	}
	emit := func(Event) {}
	if err := svc.Send(context.Background(), session.ID, "hello", emit); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- svc.SendWithModel(context.Background(), session.ID, "continue", "second", emit) }()
	<-started
	rejected := svc.SendWithModel(context.Background(), session.ID, "overlap", "third", emit)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if rejected == nil {
		t.Fatal("running session accepted a model change")
	}
	if err := svc.Send(context.Background(), session.ID, "again", emit); err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "first,second,second" {
		t.Fatalf("models: %v", models)
	}
	if len(histories[1]) != 4 || histories[1][1]["content"] != "hello" || histories[1][2]["content"] != "reply" {
		t.Fatalf("history lost during model switch: %+v", histories[1])
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model != "second" || len(loaded.Messages) != 6 {
		t.Fatalf("unexpected session: %+v", loaded)
	}
}
