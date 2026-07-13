package tools

import (
	"context"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/mark3labs/mcp-go/mcp"
)

// Delegator bundles the dependencies delegate_task needs.
type Delegator struct {
	Registry *backend.Registry
	Client   *backend.Client
}

// DelegateTaskTool returns the MCP tool definition for delegate_task.
func DelegateTaskTool() mcp.Tool {
	return mcp.NewTool("delegate_task",
		mcp.WithDescription("Send a mechanical, low-judgment task to a local or remote OpenAI-compatible model instead of doing it inline. Use for boilerplate generation, log summarization, formatting, and similar work that doesn't need architectural judgment - check classify_task_risk first if unsure."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The task/prompt to send")),
		mcp.WithString("backend", mcp.Description("Backend name to use; omit to use the first configured backend")),
		mcp.WithString("system_prompt", mcp.Description("Optional system prompt")),
		mcp.WithNumber("max_tokens", mcp.DefaultNumber(1024), mcp.Description("Maximum tokens to generate")),
		mcp.WithNumber("temperature", mcp.DefaultNumber(0.2), mcp.Description("Sampling temperature")),
		mcp.WithNumber("top_p", mcp.Description("Nucleus sampling threshold (0-1); omit to use the backend's own default")),
	)
}

// DelegateTaskHandler sends the prompt to the chosen backend and returns its reply.
func (d *Delegator) DelegateTaskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt, err := req.RequireString("prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name := req.GetString("backend", "")
	cfg, err := d.Registry.Get(name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var messages []backend.ChatMessage
	if sys := req.GetString("system_prompt", ""); sys != "" {
		messages = append(messages, backend.ChatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, backend.ChatMessage{Role: "user", Content: prompt})

	maxTokens := int(req.GetFloat("max_tokens", 1024))
	temperature := req.GetFloat("temperature", 0.2)
	topP := req.GetFloat("top_p", 0)

	reply, err := d.Client.Complete(ctx, cfg, messages, maxTokens, temperature, topP)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(reply), nil
}
