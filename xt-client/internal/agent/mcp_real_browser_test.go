package agent

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// End-to-end check against the real chrome-devtools-mcp server: the browser
// must stay reachable through the lazy activation path.
func TestRealBrowserMCPThroughLazyActivation(t *testing.T) {
	if os.Getenv("XT_REAL_BROWSER_MCP") != "1" {
		t.Skip("set XT_REAL_BROWSER_MCP=1 to run against the real browser MCP")
	}
	command := "npx"
	if runtime.GOOS == "windows" {
		command = "npx.cmd"
	}
	servers := map[string]MCPServer{
		browserMCPName: {Command: command, Args: []string{"-y", "chrome-devtools-mcp@latest"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	connections, tools, err := openMCPServers(ctx, servers)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMCPConnections(connections)
	if len(tools) == 0 {
		t.Fatal("no browser tools discovered")
	}
	t.Logf("discovered %d browser tools", len(tools))

	// Find the tool that opens a page.
	var openAlias string
	for _, definition := range tools {
		function, _ := definition["function"].(map[string]any)
		alias, _ := function["name"].(string)
		if connections[alias].names[alias] == "new_page" {
			openAlias = alias
		}
	}
	if openAlias == "" {
		t.Fatal("new_page tool missing")
	}
	args, _ := json.Marshal(map[string]any{"url": "https://example.com"})
	output, err := callMCP(ctx, connections[openAlias], openAlias, args)
	if err != nil {
		t.Fatalf("new_page failed: %v", err)
	}
	if !strings.Contains(output, "Example Domain") {
		t.Fatalf("unexpected page content: %s", output)
	}
	t.Log("real browser MCP opened the page through the lazy path")
}
