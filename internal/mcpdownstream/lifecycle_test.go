package mcpdownstream

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const helperArg = "fake-mcp-server"

// TestHelperFakeMCPServer is not a real test: when the test binary is
// spawned with helperArg (see helperServer), it serves a tiny stdio MCP
// server whose "die" tool kills the process - a real subprocess that can
// crash, to exercise reconnection.
func TestHelperFakeMCPServer(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != helperArg {
		t.Skip("helper process only")
	}
	s := server.NewMCPServer("fake-subprocess", "0.0.1")
	s.AddTool(mcp.NewTool("echo", mcp.WithString("text")), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("echo: " + req.GetString("text", "")), nil
	})
	s.AddTool(mcp.NewTool("die"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		os.Exit(3)
		return nil, nil
	})
	_ = server.ServeStdio(s)
	os.Exit(0)
}

func helperServer(name string) config.MCPServer {
	return config.MCPServer{Name: name, Command: os.Args[0], Args: []string{"-test.run=^TestHelperFakeMCPServer$", helperArg}}
}

func newEmptyManager(t *testing.T) *Manager {
	t.Helper()
	m, err := Connect(t.Context(), nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestValidateName(t *testing.T) {
	for _, bad := range []string{"", "a__b", "__"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) should fail", bad)
		}
	}
	for _, good := range []string{"codebase-memory-mcp", "a_b"} {
		if err := ValidateName(good); err != nil {
			t.Errorf("ValidateName(%q) = %v", good, err)
		}
	}
}

func TestConnectOne_RejectsNamespaceSeparatorInName(t *testing.T) {
	m := newEmptyManager(t)
	err := m.ConnectOne(t.Context(), helperServer("bad__name"))
	if err == nil || !strings.Contains(err.Error(), "__") {
		t.Fatalf("ConnectOne = %v, want a name validation error", err)
	}
}

func TestConnectOne_ReregisteringClosesPreviousConnection(t *testing.T) {
	m := newEmptyManager(t)
	if err := m.ConnectOne(t.Context(), helperServer("srv")); err != nil {
		t.Fatalf("ConnectOne: %v", err)
	}
	old := m.clients["srv"]

	if err := m.ConnectOne(t.Context(), helperServer("srv")); err != nil {
		t.Fatalf("ConnectOne (again): %v", err)
	}
	if m.clients["srv"] == old {
		t.Fatal("re-registering should replace the connection")
	}
	if _, err := old.ListTools(t.Context(), mcp.ListToolsRequest{}); err == nil {
		t.Fatal("the previous connection should have been closed, not orphaned")
	}
}

func TestCallTool_RespawnsCrashedServer(t *testing.T) {
	m := newEmptyManager(t)
	if err := m.ConnectOne(t.Context(), helperServer("srv")); err != nil {
		t.Fatalf("ConnectOne: %v", err)
	}

	// Kill the subprocess. The call itself fails (its retry dies too).
	if _, err := m.CallTool(t.Context(), "srv__die", nil); err == nil {
		t.Fatal("calling die should fail")
	}

	result, err := m.CallTool(t.Context(), "srv__echo", map[string]any{"text": "back"})
	if err != nil {
		t.Fatalf("CallTool after crash: %v", err)
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok || text.Text != "echo: back" {
		t.Fatalf("unexpected result after respawn: %+v", result.Content)
	}
}

func TestCallTool_ToolErrorDoesNotRespawn(t *testing.T) {
	m := newEmptyManager(t)
	if err := m.ConnectOne(t.Context(), helperServer("srv")); err != nil {
		t.Fatalf("ConnectOne: %v", err)
	}
	before := m.clients["srv"]

	// An unknown tool is a JSON-RPC error answer from a healthy server, not
	// a transport failure: the connection must be kept.
	if _, err := m.CallTool(t.Context(), "srv__nope", nil); err == nil {
		t.Fatal("calling an unknown tool should fail")
	}
	if m.clients["srv"] != before {
		t.Fatal("a JSON-RPC error must not trigger a respawn")
	}
}
