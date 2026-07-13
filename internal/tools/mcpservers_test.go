package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpserverregistry"
	"github.com/mark3labs/mcp-go/mcp"
)

func newTestMCPServers(t *testing.T) *MCPServers {
	t.Helper()
	reg, err := mcpserverregistry.Open(filepath.Join(t.TempDir(), "mcp-servers.db"))
	if err != nil {
		t.Fatalf("mcpserverregistry.Open: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	mgr, err := mcpdownstream.Connect(context.Background(), nil)
	if err != nil {
		t.Fatalf("mcpdownstream.Connect: %v", err)
	}
	t.Cleanup(mgr.Close)
	return &MCPServers{Registry: reg, Downstream: mgr}
}

func TestRegisterDownstreamMCPServerHandler_PersistsEvenWhenConnectFails(t *testing.T) {
	m := newTestMCPServers(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nonexistent", "command": "this-binary-does-not-exist-xyz"}

	result, err := m.RegisterDownstreamMCPServerHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterDownstreamMCPServerHandler: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected a tool error when the command can't be spawned, got: %s", toolText(t, result))
	}

	servers, err := m.Registry.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "nonexistent" {
		t.Fatalf("List() = %+v, want the definition persisted despite the connect failure", servers)
	}
}

func TestUnregisterDownstreamMCPServerHandler(t *testing.T) {
	m := newTestMCPServers(t)
	if err := m.Registry.Register(mcpserverregistry.Server{Name: "codebase-memory-mcp", Command: "cmd"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "codebase-memory-mcp"}
	result, err := m.UnregisterDownstreamMCPServerHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("UnregisterDownstreamMCPServerHandler: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", toolText(t, result))
	}

	servers, err := m.Registry.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("List() after unregister = %+v", servers)
	}
}

func TestListDownstreamMCPServersHandler(t *testing.T) {
	m := newTestMCPServers(t)
	if err := m.Registry.Register(mcpserverregistry.Server{Name: "codebase-memory-mcp", Command: "cmd"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := m.ListDownstreamMCPServersHandler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("ListDownstreamMCPServersHandler: %v", err)
	}
	text := toolText(t, result)
	if !strings.Contains(text, "codebase-memory-mcp") {
		t.Fatalf("result = %s, want it to mention the registered server", text)
	}
	if !strings.Contains(text, `"connected":false`) {
		t.Fatalf("result = %s, want connected:false since the command was never actually spawned", text)
	}
}
