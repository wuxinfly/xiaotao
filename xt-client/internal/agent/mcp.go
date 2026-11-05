package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// MCP servers are configured by the application owner, never by project content.
type mcpConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}
type mcpConnection struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Scanner
	id     int
	mu     sync.Mutex
	names  map[string]string
}

func (c *mcpConnection) close() {
	c.input.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}
func (c *mcpConnection) rpc(ctx context.Context, method string, params any, target any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-done:
				return
			default:
			}
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
		case <-done:
		}
	}()
	if err := json.NewEncoder(c.input).Encode(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params}); err != nil {
		return err
	}
	for c.output.Scan() {
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(c.output.Bytes(), &response); err != nil {
			return fmt.Errorf("MCP invalid JSON: %w", err)
		}
		if response.ID != c.id {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("MCP %d: %s", response.Error.Code, response.Error.Message)
		}
		return json.Unmarshal(response.Result, target)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := c.output.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
func openMCP(ctx context.Context) (map[string]*mcpConnection, []map[string]any, error) {
	path := os.Getenv("XIAOTAO_MCP_CONFIG")
	connections := map[string]*mcpConnection{}
	if path == "" {
		return connections, nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var config struct {
		Servers map[string]mcpConfig `json:"mcpServers"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return nil, nil, err
	}
	var definitions []map[string]any
	fail := func(err error) (map[string]*mcpConnection, []map[string]any, error) {
		seen := map[*mcpConnection]bool{}
		for _, c := range connections {
			if !seen[c] {
				seen[c] = true
				c.close()
			}
		}
		return nil, nil, err
	}
	index := 0
	serverNames := make([]string, 0, len(config.Servers))
	for name := range config.Servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		server := config.Servers[serverName]
		cmd, err := mcpCommand(ctx, server)
		if err != nil {
			return fail(err)
		}
		hideCommandWindow(cmd)
		input, err := cmd.StdinPipe()
		if err != nil {
			return fail(err)
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			input.Close()
			return fail(err)
		}
		if err = cmd.Start(); err != nil {
			input.Close()
			output.Close()
			return fail(err)
		}
		c := &mcpConnection{cmd: cmd, input: input, output: bufio.NewScanner(output), names: map[string]string{}}
		c.output.Buffer(make([]byte, 4096), 8*1024*1024)
		key := fmt.Sprintf("server_%d", index)
		connections[key] = c
		index++
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		var initialized any
		err = c.rpc(initCtx, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "xiaotao", "version": "1.0"}}, &initialized)
		cancel()
		if err != nil {
			return fail(err)
		}
		if err = json.NewEncoder(input).Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
			return fail(err)
		}
		var list struct {
			Tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Schema      map[string]any `json:"inputSchema"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		cursor := ""
		pages := 0
		for {
			pages++
			if pages > 100 {
				return fail(fmt.Errorf("MCP tool discovery exceeded 100 pages"))
			}
			params := map[string]any{}
			if cursor != "" {
				params["cursor"] = cursor
			}
			listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			list.NextCursor = ""
			list.Tools = nil
			err = c.rpc(listCtx, "tools/list", params, &list)
			cancel()
			if err != nil {
				return fail(err)
			}
			for _, tool := range list.Tools {
				name := fmt.Sprintf("mcp_%d", len(definitions))
				connections[name] = c
				definitions = append(definitions, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": serverName + "/" + tool.Name + ": " + tool.Description, "parameters": tool.Schema}})
				c.names[name] = tool.Name
			}
			if list.NextCursor == "" {
				break
			}
			if list.NextCursor == cursor {
				return fail(fmt.Errorf("MCP repeated cursor"))
			}
			cursor = list.NextCursor
		}
	}
	return connections, definitions, nil
}
func callMCP(ctx context.Context, c *mcpConnection, name string, args json.RawMessage) (string, error) {
	original, ok := c.names[name]
	if !ok {
		return "", fmt.Errorf("unknown MCP tool")
	}
	var params any
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if err := c.rpc(callCtx, "tools/call", map[string]any{"name": original, "arguments": params}, &result); err != nil {
		return "", err
	}
	var status struct {
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &status); err != nil {
		return "", err
	}
	if status.IsError {
		return string(result), fmt.Errorf("MCP tool failed: %s", result)
	}
	return string(result), nil
}

func mcpCommand(ctx context.Context, server mcpConfig) (*exec.Cmd, error) {
	if strings.TrimSpace(server.Command) == "" {
		return nil, fmt.Errorf("MCP command is empty")
	}
	if runtime.GOOS == "windows" && (strings.EqualFold(server.Command, "npx.cmd") || strings.EqualFold(server.Command, "npx")) {
		node, err := exec.LookPath("node.exe")
		if err != nil {
			return nil, err
		}
		cli := filepath.Join(filepath.Dir(node), "node_modules", "npm", "bin", "npx-cli.js")
		if _, err = os.Stat(cli); err != nil {
			return nil, fmt.Errorf("cannot locate npx-cli.js beside node.exe: %w", err)
		}
		return exec.CommandContext(ctx, node, append([]string{cli}, server.Args...)...), nil
	}
	return exec.CommandContext(ctx, server.Command, server.Args...), nil
}
