package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MCPSettings struct {
	Servers map[string]MCPServer `json:"mcpServers"`
	Source  string               `json:"source,omitempty"`
}

func validateMCP(settings MCPSettings) error {
	if len(settings.Servers) > 32 {
		return fmt.Errorf("最多配置 32 个 MCP 服务")
	}
	for name, server := range settings.Servers {
		if strings.TrimSpace(name) != name || name == "" || len(name) > 100 {
			return fmt.Errorf("服务名称不能为空、首尾不能有空格，且不能超过 100 字节")
		}
		if strings.TrimSpace(server.Command) == "" || len(server.Command) > 4096 || strings.ContainsAny(server.Command, "\x00\r\n") {
			return fmt.Errorf("%s：启动命令无效", name)
		}
		if len(server.Args) > 256 {
			return fmt.Errorf("%s：参数过多", name)
		}
		for _, arg := range server.Args {
			if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
				return fmt.Errorf("%s：参数无效", name)
			}
		}
	}
	return nil
}

func (s *Service) MCPSettings() (MCPSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dataDir, "mcp.json")
	source := "local"
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		path = os.Getenv("XIAOTAO_MCP_CONFIG")
		if path == "" {
			return MCPSettings{Servers: map[string]MCPServer{}, Source: "empty"}, nil
		}
		source = "environment"
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return MCPSettings{}, fmt.Errorf("读取 MCP 配置失败: %w", err)
	}
	if len(data) > 256*1024 {
		return MCPSettings{}, fmt.Errorf("MCP 配置超过 256 KiB")
	}
	var settings MCPSettings
	if err = json.Unmarshal(data, &settings); err != nil {
		return MCPSettings{}, fmt.Errorf("MCP 配置格式无效: %w", err)
	}
	if err = validateMCP(settings); err != nil {
		return MCPSettings{}, err
	}
	if settings.Servers == nil {
		settings.Servers = map[string]MCPServer{}
	}
	settings.Source = source
	return settings, nil
}

func (s *Service) SaveMCPSettings(settings MCPSettings) error {
	if err := validateMCP(settings); err != nil {
		return err
	}
	settings.Source = ""
	if settings.Servers == nil {
		settings.Servers = map[string]MCPServer{}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 256*1024 {
		return fmt.Errorf("MCP 配置超过 256 KiB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = os.MkdirAll(s.dataDir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dataDir, ".mcp-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(s.dataDir, "mcp.json"))
}

func closeMCPConnections(connections map[string]*mcpConnection) {
	seen := map[*mcpConnection]bool{}
	for _, c := range connections {
		if !seen[c] {
			seen[c] = true
			c.close()
		}
	}
}

func (s *Service) TestMCP(ctx context.Context, server MCPServer) ([]string, error) {
	server.Disabled = false
	if err := validateMCP(MCPSettings{Servers: map[string]MCPServer{"test": server}}); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	connections, definitions, err := openMCPServers(ctx, map[string]MCPServer{"test": server})
	if err != nil {
		return nil, err
	}
	defer closeMCPConnections(connections)
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		function := definition["function"].(map[string]any)
		alias := function["name"].(string)
		names = append(names, connections[alias].names[alias])
	}
	return names, nil
}
