package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReadBatchConcurrentAndPreservesOrder(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	active, peak := 0, 0
	gate := make(chan struct{})
	execute := func(_ context.Context, _ string, args json.RawMessage) (string, error) {
		mu.Lock()
		active++
		if active > peak {
			peak = active
		}
		if active == maxParallelReads {
			close(gate)
		}
		mu.Unlock()
		select {
		case <-gate:
		case <-time.After(3 * time.Second):
			return "", fmt.Errorf("reads did not overlap")
		}
		mu.Lock()
		active--
		mu.Unlock()
		return string(args), nil
	}
	calls := make([]toolCall, maxParallelReads)
	for i := range calls {
		calls[i] = toolCall{ID: fmt.Sprint(i), Name: "read_file", Arguments: json.RawMessage(fmt.Sprintf("%d", i))}
	}
	stats := RunStats{}
	msgs, err := svc.runReadBatch(context.Background(), execute, history, nil, calls, "turn", &stats, func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if peak != maxParallelReads || stats.ToolCalls != len(calls) || stats.FilesRead != len(calls) {
		t.Fatalf("peak=%d stats=%+v", peak, stats)
	}
	for i, msg := range msgs {
		if msg["tool_call_id"] != fmt.Sprint(i) || msg["content"] != fmt.Sprint(i) || history.Activity[i].Status != "completed" || history.Activity[i].Output != fmt.Sprint(i) {
			t.Fatalf("result %d: %+v", i, msg)
		}
	}
}

func TestLocalActivityKeepsFullCommand(t *testing.T) {
	command := `Get-ChildItem -LiteralPath xt-client\frontend\tests | Select-Object Name,Length | Select-String -Pattern 'agent'`
	input := localActivityInput("run_command", json.RawMessage(`{"command":`+strconv.Quote(command)+`}`))
	var parsed struct{Command string `json:"command"`}
	if json.Unmarshal([]byte(input), &parsed) != nil || parsed.Command != command {
		t.Fatalf("command input preview is incomplete: %s", input)
	}
	if !strings.Contains(activityDetail(json.RawMessage(`{"command":`+strconv.Quote(command)+`}`)), "...") {
		t.Fatal("expected the compact list detail to remain bounded")
	}
	if detail := activityDetail(json.RawMessage(`{"command":"go test ./..."}`)); detail != "go test ./..." {
		t.Fatalf("short command should stay unchanged: %q", detail)
	}
}

func TestReadBatchBoundaries(t *testing.T) {
	for _, name := range []string{"read_file", "list_files", "search_files"} {
		if !isParallelRead(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"write_file", "run_command", "enable_mcp_tools", "mcp_example"} {
		if isParallelRead(name) {
			t.Fatal(name)
		}
	}
}

func TestReadBatchRecordsFailuresAndCancellation(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calls := []toolCall{{ID: "first", Name: "read_file"}, {ID: "second", Name: "list_files"}}
	execute := func(_ context.Context, name string, _ json.RawMessage) (string, error) {
		if name == "read_file" {
			return "", fmt.Errorf("read failed")
		}
		return "ok", nil
	}
	stats := RunStats{}
	msgs, err := svc.runReadBatch(context.Background(), execute, history, nil, calls, "turn", &stats, func(Event) {})
	if err != nil || len(msgs) != 2 || msgs[0]["content"] != "read failed" || history.Activity[0].Status != "failed" || history.Activity[1].Status != "completed" {
		t.Fatalf("messages=%+v activities=%+v error=%v", msgs, history.Activity, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.runReadBatch(ctx, execute, history, nil, calls, "turn", &stats, func(Event) {})
	if err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestReadBatchBeforeWritePreservesToolOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	history, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{replies: []*completion{{ToolCalls: []toolCall{
		{ID: "read-a", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
		{ID: "read-b", Name: "read_file", Arguments: json.RawMessage(`{"path":"b.txt"}`)},
		{ID: "write-a", Name: "write_file", Arguments: json.RawMessage(`{"path":"a.txt","content":"new"}`)},
	}}, {Content: "done"}}}
	if err := svc.loop(context.Background(), workspace, model, history, nil, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(data) != "new" {
		t.Fatalf("write did not follow reads: %q %v", data, err)
	}
	for i, id := range []string{"read-a", "read-b", "write-a"} {
		if history.Messages[i+1].ToolCallID != id {
			t.Fatalf("tool order: %+v", history.Messages)
		}
	}
}
