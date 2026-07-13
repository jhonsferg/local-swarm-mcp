package mcpdownstream

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// fakeServer builds an in-process MCP server with one tool, "echo", so
// tests can exercise Manager without spawning a real subprocess.
func fakeServer(t *testing.T) *client.Client {
	t.Helper()
	s := server.NewMCPServer("fake", "0.0.1")
	echoTool := mcp.NewTool("echo",
		mcp.WithDescription("Echoes its input"),
		mcp.WithString("text", mcp.Required()),
	)
	s.AddTool(echoTool, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, _ := req.RequireString("text")
		return mcp.NewToolResultText("echo: " + text), nil
	})

	c, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	return c
}

func newTestManager(t *testing.T, name string) *Manager {
	t.Helper()
	m := &Manager{
		clients: make(map[string]*client.Client),
		tools:   make(map[string][]mcp.Tool),
	}
	if err := m.Attach(t.Context(), name, fakeServer(t)); err != nil {
		t.Fatalf("attach: %v", err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestManager_ListAllTools(t *testing.T) {
	m := newTestManager(t, "srv1")

	tools := m.ListAllTools(nil)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d: %+v", len(tools), tools)
	}
	if tools[0].QualifiedName != "srv1__echo" {
		t.Fatalf("unexpected qualified name: %q", tools[0].QualifiedName)
	}
	if tools[0].ServerName != "srv1" {
		t.Fatalf("unexpected server name: %q", tools[0].ServerName)
	}
}

func TestManager_ListAllTools_Filtered(t *testing.T) {
	m := newTestManager(t, "srv1")

	if got := m.ListAllTools([]string{"nope"}); len(got) != 0 {
		t.Fatalf("expected 0 tools for a non-matching filter, got %d", len(got))
	}
	if got := m.ListAllTools([]string{"srv1"}); len(got) != 1 {
		t.Fatalf("expected 1 tool for a matching filter, got %d", len(got))
	}
}

func TestManager_CallTool(t *testing.T) {
	m := newTestManager(t, "srv1")

	result, err := m.CallTool(t.Context(), "srv1__echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok || text.Text != "echo: hi" {
		t.Fatalf("unexpected result: %+v", result.Content)
	}
}

func TestManager_CallTool_UnknownServer(t *testing.T) {
	m := newTestManager(t, "srv1")
	if _, err := m.CallTool(t.Context(), "nope__echo", nil); err == nil {
		t.Fatal("expected an error for an unknown server")
	}
}

func TestManager_CallTool_NotNamespaced(t *testing.T) {
	m := newTestManager(t, "srv1")
	if _, err := m.CallTool(t.Context(), "echo", nil); err == nil {
		t.Fatal("expected an error for a non-namespaced tool name")
	}
}

func TestConnect_EmptyServerList(t *testing.T) {
	m, err := Connect(t.Context(), nil)
	if err != nil {
		t.Fatalf("Connect with no servers should not error: %v", err)
	}
	if len(m.ListAllTools(nil)) != 0 {
		t.Fatal("expected no tools with no configured servers")
	}
	m.Close()
}
