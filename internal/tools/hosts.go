package tools

import (
	"context"

	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
	"github.com/mark3labs/mcp-go/mcp"
)

// Hosts bundles the dependencies the host-discovery tools need. Available
// both when running as the persistent HTTP daemon and in a plain -transport
// stdio session (each backed by its own in-process poller, sharing the same
// persisted store) - hosts registered here are polled in the background and
// their models become usable as "<host>/<model>" backends automatically.
type Hosts struct {
	Registry *hostregistry.Registry
	// OnRegistered triggers an immediate poll of a newly registered host,
	// instead of waiting for the next tick.
	OnRegistered func(ctx context.Context, host hostregistry.Host)
}

// RegisterBackendHostTool returns the MCP tool definition for
// register_backend_host.
func RegisterBackendHostTool() mcp.Tool {
	return mcp.NewTool("register_backend_host",
		mcp.WithDescription("Register a new inference host (e.g. a remote Ollama instance on a desktop GPU, a DGX Spark, an AMD AI box) for background model discovery - no config file edit or restart needed. Its models are polled periodically and become usable as \"<host>/<model>\" backend names once discovered; check list_backend_hosts to see what's been found."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Short name for this host, e.g. \"rx9070\" or \"dgx-spark\"")),
		mcp.WithString("base_url", mcp.Required(), mcp.Description("Ollama root URL, e.g. http://192.168.18.29:11434 (no /v1 suffix - that's derived automatically per discovered model)")),
		mcp.WithString("api_key", mcp.Description("API key for this host, if any")),
	)
}

// RegisterBackendHostHandler persists a new host and kicks off an
// immediate poll.
func (h *Hosts) RegisterBackendHostHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	baseURL, err := req.RequireString("base_url")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	apiKey := req.GetString("api_key", "")

	host := hostregistry.Host{Name: name, BaseURL: baseURL, APIKey: apiKey}
	if err := h.Registry.RegisterHost(host); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if h.OnRegistered != nil {
		h.OnRegistered(ctx, host)
	}
	return mcp.NewToolResultText("registered host " + name + "; polling for models now"), nil
}

// UnregisterBackendHostTool returns the MCP tool definition for
// unregister_backend_host.
func UnregisterBackendHostTool() mcp.Tool {
	return mcp.NewTool("unregister_backend_host",
		mcp.WithDescription("Remove a registered inference host and stop polling it. Its discovered \"<host>/<model>\" backends stop being usable."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Host name to remove")),
	)
}

// UnregisterBackendHostHandler removes a host.
func (h *Hosts) UnregisterBackendHostHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := h.Registry.UnregisterHost(name); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("unregistered host " + name), nil
}

// ListBackendHostsTool returns the MCP tool definition for
// list_backend_hosts.
func ListBackendHostsTool() mcp.Tool {
	return mcp.NewTool("list_backend_hosts",
		mcp.WithDescription("List every registered inference host, whether it's currently reachable (Up), and the models discovered there so far. Check this before spawning work on a host you haven't confirmed is online - a powered-off host stays registered but Up will be false and its backends won't be dispatched to."),
	)
}

// ListBackendHostsHandler returns the live status of every registered host.
func (h *Hosts) ListBackendHostsHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return jsonResult(h.Registry.Status())
}
