package tools

import (
	"context"

	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/orchestrator"
	"github.com/mark3labs/mcp-go/mcp"
)

// Agents bundles the dependencies spawn_agent_task and
// list_available_agent_tools need.
type Agents struct {
	TaskRegistry *orchestrator.TaskRegistry
	Downstream   *mcpdownstream.Manager
}

// SpawnAgentTaskTool returns the MCP tool definition for spawn_agent_task.
func SpawnAgentTaskTool() mcp.Tool {
	return mcp.NewTool("spawn_agent_task",
		mcp.WithDescription("Start a tool-using agent in the background: the model can call tools discovered from configured downstream MCP servers (see list_available_agent_tools) in a loop before producing a final answer, the same way a host application's own agents work. Requires a backend/model that actually supports structured tool-calling (verified working: llama3.1:8b via Ollama; verified NOT working: qwen2.5-coder 1.5b/7b, which print tool-call-shaped text instead of using the real mechanism) - check with a small test before relying on this for an unfamiliar model. Poll with task_status or block with wait_task; the result includes a transcript of every tool call made."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The task/prompt to send")),
		mcp.WithString("backend", mcp.Description("Backend name to use; omit to use the first configured backend. Must be a tool-calling-capable model.")),
		mcp.WithString("system_prompt", mcp.Description("Optional system prompt")),
		mcp.WithArray("mcp_servers", mcp.Description("Names of configured downstream MCP servers whose tools this agent may use; omit to allow all configured servers")),
		mcp.WithNumber("max_iterations", mcp.DefaultNumber(float64(orchestrator.DefaultMaxIterations)), mcp.Description("Maximum tool-call round-trips before giving up")),
		mcp.WithNumber("max_tokens", mcp.DefaultNumber(1024), mcp.Description("Maximum tokens to generate per completion")),
		mcp.WithNumber("temperature", mcp.DefaultNumber(0.2), mcp.Description("Sampling temperature")),
		mcp.WithNumber("top_p", mcp.Description("Nucleus sampling threshold (0-1); omit to use the backend's own default")),
	)
}

// SpawnAgentTaskHandler starts a background tool-using agent task and
// returns its ID.
func (a *Agents) SpawnAgentTaskHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt, err := req.RequireString("prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	backendName := req.GetString("backend", "")
	systemPrompt := req.GetString("system_prompt", "")
	serverFilter := req.GetStringSlice("mcp_servers", nil)
	maxIterations := int(req.GetFloat("max_iterations", float64(orchestrator.DefaultMaxIterations)))
	maxTokens := int(req.GetFloat("max_tokens", 1024))
	temperature := req.GetFloat("temperature", 0.2)
	topP := req.GetFloat("top_p", 0)

	id, err := a.TaskRegistry.SpawnAgent(backendName, systemPrompt, prompt, a.Downstream, serverFilter, maxIterations, maxTokens, temperature, topP)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(id), nil
}

// ListAvailableAgentToolsTool returns the MCP tool definition for
// list_available_agent_tools.
func ListAvailableAgentToolsTool() mcp.Tool {
	return mcp.NewTool("list_available_agent_tools",
		mcp.WithDescription("List every tool discovered from configured downstream MCP servers, namespaced as <server>__<tool>, that a spawn_agent_task call could invoke. Check this before spawning an agent to know what it can actually do."),
		mcp.WithArray("mcp_servers", mcp.Description("Restrict the listing to these downstream server names; omit to list all configured servers")),
	)
}

// ListAvailableAgentToolsHandler lists discovered downstream tools.
func (a *Agents) ListAvailableAgentToolsHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serverFilter := req.GetStringSlice("mcp_servers", nil)
	tools := a.Downstream.ListAllTools(serverFilter)

	type toolSummary struct {
		Name        string `json:"name"`
		Server      string `json:"server"`
		Description string `json:"description,omitempty"`
	}
	out := make([]toolSummary, 0, len(tools))
	for _, t := range tools {
		out = append(out, toolSummary{Name: t.QualifiedName, Server: t.ServerName, Description: t.Tool.Description})
	}
	return jsonResult(out)
}
