package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestMCPFixture(t *testing.T) {
	if os.Getenv("XT_MCP_FIXTURE") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		if request.ID == 0 {
			continue
		}
		var result any = map[string]any{}
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "fixture echo", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			if os.Getenv("XT_MCP_HANG") == "1" {
				time.Sleep(time.Minute)
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "fixture result"}}}
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}
	os.Exit(0)
}
func TestMCPDiscoveryAndCall(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	data, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"fixture": map[string]any{"command": os.Args[0], "args": []string{"-test.run=^TestMCPFixture$"}}}})
	path := t.TempDir() + "/mcp.json"
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XIAOTAO_MCP_CONFIG", path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connections, definitions, err := openMCP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for name, c := range connections {
			if name == "server_0" {
				c.close()
			}
		}
	}()
	if len(definitions) != 1 {
		t.Fatalf("tools: %v", definitions)
	}
	output, err := callMCP(ctx, connections["mcp_0"], "mcp_0", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Content []struct{ Text string } }
	if err = json.Unmarshal([]byte(output), &result); err != nil || len(result.Content) != 1 || result.Content[0].Text != "fixture result" {
		t.Fatalf("result: %s %v", output, err)
	}
}
func TestMCPConfigFailure(t *testing.T) {
	t.Setenv("XIAOTAO_MCP_CONFIG", t.TempDir()+"/missing.json")
	if _, _, err := openMCP(context.Background()); err == nil {
		t.Fatal("missing config accepted")
	}
}

func TestMCPCancel(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	t.Setenv("XT_MCP_HANG", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, err := mcpCommand(ctx, mcpConfig{Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := cmd.StdinPipe()
	output, _ := cmd.StdoutPipe()
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &mcpConnection{cmd: cmd, input: input, output: bufio.NewScanner(output)}
	defer c.close()
	callCtx, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stop()
	var result any
	if err = c.rpc(callCtx, "tools/call", map[string]any{}, &result); err == nil {
		t.Fatal("cancellation ignored")
	}
	if callCtx.Err() == nil {
		t.Fatalf("unexpected failure: %v", err)
	}
}
