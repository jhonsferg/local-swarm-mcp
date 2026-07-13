package tools

import (
	"context"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpserverregistry"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPServers bundles the dependencies the downstream-MCP-server discovery
// tools need. Available both when running as the persistent HTTP daemon and
// in a plain -transport stdio session - registering here both persists the
// definition and connects it immediately, so it's usable by spawn_agent_task
// without a restart.
type MCPServers struct {
	Registry   *mcpserverregistry.Registry
	Downstream *mcpdownstream.Manager
}

// RegisterDownstreamMCPServerTool returns the MCP tool definition for
// register_downstream_mcp_server.
func RegisterDownstreamMCPServerTool() mcp.Tool {
	return mcp.NewTool("register_downstream_mcp_server",
		mcp.WithDescription("Register a downstream MCP server (e.g. codebase-memory-mcp) that tool-using agents (spawn_agent_task) can invoke - no config file edit or restart needed. Spawned as a local stdio subprocess and connected immediately; check list_available_agent_tools to see what it exposes."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Short name for this server, e.g. \"codebase-memory-mcp\"")),
		mcp.WithString("command", mcp.Required(), mcp.Description("Executable path to spawn")),
		mcp.WithArray("args", mcp.Description("Command-line arguments, if any")),
	)
}

// RegisterDownstreamMCPServerHandler persists a new downstream server and
// connects it immediately.
func (m *MCPServers) RegisterDownstreamMCPServerHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command, err := req.RequireString("command")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := req.GetStringSlice("args", nil)

	srv := mcpserverregistry.Server{Name: name, Command: command, Args: args}
	if err := m.Registry.Register(srv); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := m.Downstream.ConnectOne(ctx, config.MCPServer{Name: name, Command: command, Args: args}); err != nil {
		return mcp.NewToolResultError("registered but failed to connect: " + err.Error()), nil
	}
	return mcp.NewToolResultText("registered and connected downstream MCP server " + name), nil
}

// UnregisterDownstreamMCPServerTool returns the MCP tool definition for
// unregister_downstream_mcp_server.
func UnregisterDownstreamMCPServerTool() mcp.Tool {
	return mcp.NewTool("unregister_downstream_mcp_server",
		mcp.WithDescription("Remove a registered downstream MCP server and disconnect it. Its tools stop being usable by spawn_agent_task."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Server name to remove")),
	)
}

// UnregisterDownstreamMCPServerHandler disconnects and removes a
// downstream server.
func (m *MCPServers) UnregisterDownstreamMCPServerHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := m.Registry.Unregister(name); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	m.Downstream.Detach(name)
	return mcp.NewToolResultText("unregistered downstream MCP server " + name), nil
}

// ListDownstreamMCPServersTool returns the MCP tool definition for
// list_downstream_mcp_servers.
func ListDownstreamMCPServersTool() mcp.Tool {
	return mcp.NewTool("list_downstream_mcp_servers",
		mcp.WithDescription("List every registered downstream MCP server and whether it's currently connected."),
	)
}

// ListDownstreamMCPServersHandler reports every registered server and its
// live connection state.
func (m *MCPServers) ListDownstreamMCPServersHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	servers, err := m.Registry.List()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	connected := make(map[string]bool)
	for _, name := range m.Downstream.Names() {
		connected[name] = true
	}

	type status struct {
		Name      string   `json:"name"`
		Command   string   `json:"command"`
		Args      []string `json:"args,omitempty"`
		Connected bool     `json:"connected"`
	}
	out := make([]status, 0, len(servers))
	for _, s := range servers {
		out = append(out, status{Name: s.Name, Command: s.Command, Args: s.Args, Connected: connected[s.Name]})
	}
	return jsonResult(out)
}
