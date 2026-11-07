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
	"time"
)

func TestConcurrentSessionsStopIndependently(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(body.Tools) == 0 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"确认请求。"}}]}`))
			return
		}
		entered <- body.Messages[0].Content
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer server.Close()
	defer close(release)
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	project := t.TempDir()
	a, err := svc.Create(Options{ProjectPath: project, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(Options{ProjectPath: project, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	doneA, doneB := make(chan error, 1), make(chan error, 1)
	go func() { doneA <- svc.Send(context.Background(), a.ID, "task alpha", func(Event) {}) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("A did not start")
	}
	go func() { doneB <- svc.Send(context.Background(), b.ID, "task beta", func(Event) {}) }()
	select {
	case prompt := <-entered:
		if !strings.Contains(prompt, "task alpha") {
			t.Fatal("missing concurrent task context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("B blocked behind A")
	}
	if err := svc.Send(context.Background(), a.ID, "duplicate", func(Event) {}); err == nil {
		t.Fatal("same session accepted simultaneous send")
	}
	if !svc.Cancel(b.ID) {
		t.Fatal("cancel failed")
	}
	select {
	case err := <-doneB:
		if err == nil {
			t.Fatal("B not canceled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("B did not stop")
	}
	select {
	case <-doneA:
		t.Fatal("stopping B affected A")
	default:
	}
	release <- struct{}{}
	select {
	case err := <-doneA:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("A did not finish")
	}
}

func TestSharedFileRejectsStaleWriteAndAllowsReread(t *testing.T) {
	root := t.TempDir()
	a, _ := OpenWorkspace(root)
	b, _ := OpenWorkspace(root)
	target := filepath.Join(root, "shared.txt")
	if err := a.Write("shared.txt", "original", false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read("shared.txt", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := a.Write("shared.txt", "alpha", false); err != nil {
		t.Fatal(err)
	}
	for _, appendMode := range []bool{false, true} {
		if err := b.Write("shared.txt", "stale", appendMode); err == nil || !strings.Contains(err.Error(), "文件冲突") {
			t.Fatalf("stale write: %v", err)
		}
	}
	data, _ := os.ReadFile(target)
	if string(data) != "alpha" {
		t.Fatalf("overwritten: %q", data)
	}
	if _, err := b.Read("shared.txt", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := b.Write("shared.txt", "alpha + beta", false); err != nil {
		t.Fatal(err)
	}
	c, _ := OpenWorkspace(root)
	if err := c.Write("shared.txt", "unread", false); err == nil {
		t.Fatal("overwrote unread existing file")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := b.Write("shared.txt", "recreated", false); err == nil {
		t.Fatal("recreated file deleted since read")
	}
	if _, err := b.Read("shared.txt", 1, 1); !os.IsNotExist(err) {
		t.Fatalf("expected confirmed deletion: %v", err)
	}
	if err := b.Write("shared.txt", "recreated after checking", false); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentFileToolsOnlyOneStaleSnapshotCanWin(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "shared.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := OpenWorkspace(root)
	b, _ := OpenWorkspace(root)
	for _, w := range []*Workspace{a, b} {
		if _, err := w.Read("shared.txt", 1, 1); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, w := range []*Workspace{a, b} {
		go func(w *Workspace, content string) { <-start; results <- w.Write("shared.txt", content, false) }(w, []string{"alpha", "beta"}[i])
	}
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("expected one success and one conflict: %v, %v", first, second)
	}
}
