// Package mcpdownstream lets local-swarm-mcp act as an MCP client toward
// other configured MCP servers (e.g. codebase-memory-mcp), so a tool-using
// agent task can discover and invoke their tools the same way an MCP host
// like Claude Code invokes its own tools.
package mcpdownstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// namespaceSep separates a configured server's name from a tool's own name
// in the namespaced tool identifiers ListAllTools/CallTool use, so two
// downstream servers can each expose a tool with the same short name.
const namespaceSep = "__"

// ClientVersion is reported to downstream servers in the MCP initialize
// handshake; main sets it to the build version.
var ClientVersion = "dev"

// connectTimeout bounds starting and initializing one downstream server, so
// a command that never speaks MCP can't hang a registration forever.
const connectTimeout = 30 * time.Second

// Manager holds a live MCP client connection to each configured downstream
// server.
type Manager struct {
	mu      sync.RWMutex
	clients map[string]*client.Client
	tools   map[string][]mcp.Tool       // server name -> its tools
	configs map[string]config.MCPServer // server name -> how to respawn it, for servers started by command
}

// ValidateName rejects server names that can't be addressed unambiguously
// in "<server>__<tool>" tool names.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("downstream MCP server name must not be empty")
	}
	if strings.Contains(name, namespaceSep) {
		return fmt.Errorf("downstream MCP server name %q must not contain %q (it separates server and tool names)", name, namespaceSep)
	}
	return nil
}

// Connect spawns and initializes a client for every configured server,
// discovering its tools. A server that fails to start or initialize is
// skipped with its error returned in the aggregate, rather than aborting
// the whole manager - one misconfigured downstream server shouldn't take
// down every other one.
func Connect(ctx context.Context, servers []config.MCPServer) (*Manager, error) {
	m := &Manager{
		clients: make(map[string]*client.Client, len(servers)),
		tools:   make(map[string][]mcp.Tool, len(servers)),
		configs: make(map[string]config.MCPServer, len(servers)),
	}

	var errs []string
	for _, srv := range servers {
		if err := m.connectOne(ctx, srv); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", srv.Name, err))
		}
	}

	if len(errs) > 0 {
		return m, fmt.Errorf("failed to connect to %d downstream server(s): %s", len(errs), strings.Join(errs, "; "))
	}
	return m, nil
}

func (m *Manager) connectOne(ctx context.Context, srv config.MCPServer) error {
	if err := ValidateName(srv.Name); err != nil {
		return err
	}
	c, err := client.NewStdioMCPClient(srv.Command, nil, srv.Args...)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := m.Attach(ctx, srv.Name, c); err != nil {
		return err
	}
	m.mu.Lock()
	if m.configs == nil {
		m.configs = make(map[string]config.MCPServer)
	}
	m.configs[srv.Name] = srv
	m.mu.Unlock()
	return nil
}

// ConnectOne connects a single downstream server at runtime - e.g. one
// just registered dynamically, without a restart. Exported so registering
// a new server doesn't need the whole batch Connect path.
func (m *Manager) ConnectOne(ctx context.Context, srv config.MCPServer) error {
	return m.connectOne(ctx, srv)
}

// Detach closes and forgets a downstream server's connection, if any.
func (m *Manager) Detach(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[name]; ok {
		_ = c.Close()
		delete(m.clients, name)
	}
	delete(m.tools, name)
	delete(m.configs, name)
}

// Names returns the names of every currently connected downstream server.
func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.clients))
	for name := range m.clients {
		out = append(out, name)
	}
	return out
}

// Attach initializes an already-constructed client and records its
// discovered tools under name. Exposed (rather than folded into
// connectOne) so callers - including tests - can attach a client built any
// way they like, e.g. an in-process client (client.NewInProcessClient)
// instead of a spawned subprocess.
func (m *Manager) Attach(ctx context.Context, name string, c *client.Client) error {
	if err := ValidateName(name); err != nil {
		_ = c.Close()
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "local-swarm-mcp", Version: ClientVersion}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		_ = c.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	m.mu.Lock()
	// Re-registering a name replaces its connection: close the old one
	// rather than orphaning its subprocess.
	if old, ok := m.clients[name]; ok && old != c {
		_ = old.Close()
	}
	m.clients[name] = c
	m.tools[name] = toolsResult.Tools
	m.mu.Unlock()

	return nil
}

// Close shuts down every downstream client connection.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		_ = c.Close()
	}
}

// NamespacedTool pairs a downstream tool with the server it came from and
// the namespaced identifier (server__tool) used to address it.
type NamespacedTool struct {
	ServerName    string
	QualifiedName string
	Tool          mcp.Tool
}

// ListAllTools returns every discovered tool from every connected server,
// namespaced as "<server>__<tool>" to avoid collisions between servers.
// When serverFilter is non-empty, only those server names are included.
func (m *Manager) ListAllTools(serverFilter []string) []NamespacedTool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	allow := toSet(serverFilter)
	var out []NamespacedTool
	for serverName, tools := range m.tools {
		if len(allow) > 0 && !allow[serverName] {
			continue
		}
		for _, t := range tools {
			out = append(out, NamespacedTool{
				ServerName:    serverName,
				QualifiedName: serverName + namespaceSep + t.Name,
				Tool:          t,
			})
		}
	}
	return out
}

// CallTool invokes qualifiedName (as produced by ListAllTools) with args
// against the owning downstream server.
func (m *Manager) CallTool(ctx context.Context, qualifiedName string, args map[string]any) (*mcp.CallToolResult, error) {
	serverName, toolName, err := splitQualified(qualifiedName)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	c, ok := m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown downstream server %q", serverName)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args
	result, err := c.CallTool(ctx, req)
	var transportErr *transport.Error
	if err == nil || ctx.Err() != nil || !errors.As(err, &transportErr) {
		return result, err
	}

	// A transport-level failure - not a JSON-RPC error answer, and not a
	// tool's own failure (result.IsError) - means the subprocess is gone.
	// Respawn it once from its stored command and retry, instead of
	// leaving the server dead until the whole process restarts.
	m.mu.RLock()
	srv, respawnable := m.configs[serverName]
	m.mu.RUnlock()
	if !respawnable {
		return nil, err
	}
	if reconnectErr := m.connectOne(ctx, srv); reconnectErr != nil {
		return nil, fmt.Errorf("%w (reconnecting %q also failed: %w)", err, serverName, reconnectErr)
	}
	m.mu.RLock()
	c, ok = m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown downstream server %q", serverName)
	}
	return c.CallTool(ctx, req)
}

func splitQualified(qualifiedName string) (server, tool string, err error) {
	server, tool, found := strings.Cut(qualifiedName, namespaceSep)
	if !found {
		return "", "", fmt.Errorf("tool name %q is not namespaced as <server>__<tool>", qualifiedName)
	}
	return server, tool, nil
}

func toSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}
