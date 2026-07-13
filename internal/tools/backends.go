// Package tools implements the MCP tool definitions and handlers exposed by
// local-swarm-mcp: task delegation, context compaction, a scratch store,
// token estimation, and a task-risk classifier.
package tools

import (
	"context"
	"encoding/json"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/mark3labs/mcp-go/mcp"
)

// Backends bundles the dependencies the backend-related tools need.
type Backends struct {
	Registry *backend.Registry
}

// ListBackendsTool returns the MCP tool definition for list_backends.
func ListBackendsTool() mcp.Tool {
	return mcp.NewTool("list_backends",
		mcp.WithDescription("List configured OpenAI-compatible inference backends (name, base_url, model)."),
	)
}

// ListBackendsHandler returns the configured backends as JSON.
func (b *Backends) ListBackendsHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := json.Marshal(b.Registry.List())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

// HealthCheckTool returns the MCP tool definition for health_check.
func HealthCheckTool() mcp.Tool {
	return mcp.NewTool("health_check",
		mcp.WithDescription("Probe whether a backend (or all backends, if omitted) is reachable before delegating work to it."),
		mcp.WithString("backend", mcp.Description("Backend name to check; omit to check all configured backends")),
	)
}

// HealthCheckHandler probes one or all configured backends.
func (b *Backends) HealthCheckHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := req.GetString("backend", "")

	var targets []backend.HealthStatus
	if name == "" {
		for _, cfg := range b.Registry.List() {
			targets = append(targets, backend.Check(ctx, cfg))
		}
	} else {
		cfg, err := b.Registry.Get(name)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		targets = append(targets, backend.Check(ctx, cfg))
	}

	out, err := json.Marshal(targets)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}
