package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPSettingsPersistenceAndPriority(t *testing.T) {
	external := filepath.Join(t.TempDir(), "external.json")
	original := []byte(`{"mcpServers":{"legacy":{"command":"legacy"}}}`)
	if err := os.WriteFile(external, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XIAOTAO_MCP_CONFIG", external)
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	settings, err := svc.MCPSettings()
	if err != nil || settings.Source != "environment" || settings.Servers["legacy"].Command != "legacy" {
		t.Fatalf("legacy: %+v %v", settings, err)
	}
	settings = MCPSettings{Servers: map[string]MCPServer{"local": {Command: "local", Args: []string{"a b", ""}, Disabled: true}}}
	if err = svc.SaveMCPSettings(settings); err != nil {
		t.Fatal(err)
	}
	reread, err := svc.MCPSettings()
	if err != nil || reread.Source != "local" || !reread.Servers["local"].Disabled || len(reread.Servers["local"].Args) != 2 {
		t.Fatalf("local: %+v %v", reread, err)
	}
	if err = svc.SaveMCPSettings(MCPSettings{Servers: map[string]MCPServer{"bad": {}}}); err == nil {
		t.Fatal("invalid config saved")
	}
	reread, _ = svc.MCPSettings()
	if len(reread.Servers) != 1 || reread.Servers["local"].Command != "local" {
		t.Fatal("invalid save clobbered config")
	}
	// An empty local file explicitly overrides a legacy environment config.
	if err = svc.SaveMCPSettings(MCPSettings{}); err != nil {
		t.Fatal(err)
	}
	reread, err = svc.MCPSettings()
	if err != nil || len(reread.Servers) != 0 || reread.Source != "local" {
		t.Fatalf("empty override: %+v %v", reread, err)
	}
	unchanged, _ := os.ReadFile(external)
	if string(unchanged) != string(original) {
		t.Fatal("modified external file")
	}
	other := NewService(t.TempDir(), "", nil, nil, nil)
	isolated, _ := other.MCPSettings()
	if isolated.Source != "environment" {
		t.Fatal("settings leaked between data directories")
	}
}

func TestMCPSettingsErrorsAndDisabled(t *testing.T) {
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	settings, err := svc.MCPSettings()
	if err != nil || settings.Source != "empty" {
		t.Fatalf("empty: %+v %v", settings, err)
	}
	connections, tools, err := openMCPServers(context.Background(), map[string]MCPServer{"off": {Command: "does-not-exist", Disabled: true}})
	if err != nil || len(connections) != 0 || len(tools) != 0 {
		t.Fatalf("disabled server started: %v", err)
	}
	if err = os.WriteFile(filepath.Join(svc.dataDir, "mcp.json"), []byte("bad json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.MCPSettings(); err == nil {
		t.Fatal("corrupt config accepted")
	}
}

func TestMCPSettingsConnectionTestDoesNotSave(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	tools, err := svc.TestMCP(context.Background(), MCPServer{Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}, Disabled: true})
	if err != nil || len(tools) != 1 || tools[0] != "echo" {
		t.Fatalf("test: %v %v", tools, err)
	}
	settings, _ := svc.MCPSettings()
	if settings.Source != "empty" {
		t.Fatal("connection test persisted draft")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = svc.TestMCP(ctx, MCPServer{Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}}); err == nil {
		t.Fatal("cancel ignored")
	}
	_, _, err = openMCPServers(context.Background(), map[string]MCPServer{"missing": {Command: filepath.Join(t.TempDir(), "missing.exe")}})
	if err == nil {
		t.Fatal("missing executable accepted")
	}
	if needs, ok := needsApproval(nil, "mcp_0", json.RawMessage(`{}`)); !ok || needs == "" {
		t.Fatal("MCP approval lost")
	}
}
