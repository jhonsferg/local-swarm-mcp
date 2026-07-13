package tools

import (
	"context"
	"fmt"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/mark3labs/mcp-go/mcp"
)

// Compactor bundles the dependencies compact_context needs. It reuses the
// same backend/client as Delegator - compaction is just a delegated task
// with a fixed summarization prompt.
type Compactor struct {
	Registry *backend.Registry
	Client   *backend.Client
}

// CompactContextTool returns the MCP tool definition for compact_context.
func CompactContextTool() mcp.Tool {
	return mcp.NewTool("compact_context",
		mcp.WithDescription("Summarize/shrink a block of text (e.g. a large tool output or subagent report) using a local model, instead of letting it sit in Claude's own context uncompacted. Call estimate_tokens first to judge whether it's worth it."),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to compact")),
		mcp.WithNumber("target_tokens", mcp.DefaultNumber(500), mcp.Description("Approximate target length in tokens")),
		mcp.WithString("preserve_instructions", mcp.Description("Optional extra instructions about what must be preserved (e.g. specific facts, numbers, file paths)")),
		mcp.WithString("backend", mcp.Description("Backend name to use; omit to use the first configured backend")),
	)
}

// CompactContextHandler asks a backend to summarize text down to roughly
// target_tokens while preserving anything called out in preserve_instructions.
func (c *Compactor) CompactContextHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := req.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name := req.GetString("backend", "")
	cfg, err := c.Registry.Get(name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	targetTokens := int(req.GetFloat("target_tokens", 500))
	preserve := req.GetString("preserve_instructions", "")

	systemPrompt := "You compress text for another AI system's working memory. " +
		"Preserve concrete facts, numbers, file paths, and decisions; drop narration and filler."
	userPrompt := fmt.Sprintf("Summarize the following text in roughly %d tokens.", targetTokens)
	if preserve != "" {
		userPrompt += " Must preserve: " + preserve
	}
	userPrompt += "\n\n---\n\n" + text

	messages := []backend.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reply, err := c.Client.Complete(ctx, cfg, messages, targetTokens*2, 0.1)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(reply), nil
}
