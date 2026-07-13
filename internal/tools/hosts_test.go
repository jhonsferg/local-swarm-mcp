package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
	"github.com/mark3labs/mcp-go/mcp"
)

func newTestHosts(t *testing.T) *Hosts {
	t.Helper()
	reg, err := hostregistry.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatalf("hostregistry.Open: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	return &Hosts{Registry: reg}
}

func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty result content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}

func TestRegisterBackendHostHandler_TriggersOnRegistered(t *testing.T) {
	h := newTestHosts(t)
	var got hostregistry.Host
	called := false
	h.OnRegistered = func(_ context.Context, host hostregistry.Host) {
		called = true
		got = host
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "rx9070", "base_url": "http://192.168.18.29:11434"}

	result, err := h.RegisterBackendHostHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterBackendHostHandler: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", toolText(t, result))
	}
	if !called {
		t.Fatal("OnRegistered was not called")
	}
	if got.Name != "rx9070" || got.BaseURL != "http://192.168.18.29:11434" {
		t.Fatalf("OnRegistered called with %+v", got)
	}

	hosts, err := h.Registry.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "rx9070" {
		t.Fatalf("Hosts() = %+v", hosts)
	}
}

func TestRegisterBackendHostHandler_RequiresName(t *testing.T) {
	h := newTestHosts(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"base_url": "http://192.168.18.29:11434"}

	result, err := h.RegisterBackendHostHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterBackendHostHandler: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected a tool error when name is missing")
	}
}

func TestUnregisterBackendHostHandler(t *testing.T) {
	h := newTestHosts(t)
	if err := h.Registry.RegisterHost(hostregistry.Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "rx9070"}
	result, err := h.UnregisterBackendHostHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("UnregisterBackendHostHandler: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", toolText(t, result))
	}

	hosts, err := h.Registry.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("Hosts() after unregister = %+v", hosts)
	}
}

func TestListBackendHostsHandler(t *testing.T) {
	h := newTestHosts(t)
	if err := h.Registry.RegisterHost(hostregistry.Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	result, err := h.ListBackendHostsHandler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("ListBackendHostsHandler: %v", err)
	}
	text := toolText(t, result)
	if !strings.Contains(text, "rx9070") || !strings.Contains(text, "192.168.18.29") {
		t.Fatalf("result = %s, want it to mention the registered host", text)
	}
}
