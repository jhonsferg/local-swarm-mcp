package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// EstimateTokensTool returns the MCP tool definition for estimate_tokens.
func EstimateTokensTool() mcp.Tool {
	return mcp.NewTool("estimate_tokens",
		mcp.WithDescription("Rough token-count estimate for a block of text (heuristic, not a real tokenizer). Use before compact_context to decide whether compaction is worth the round-trip."),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to estimate")),
	)
}

// EstimateTokensHandler returns a heuristic token-count estimate for the
// given text.
func EstimateTokensHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := req.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	estimate := estimateTokens(text)
	return mcp.NewToolResultText(fmt.Sprintf("~%d tokens (heuristic estimate over %d chars)", estimate, len(text))), nil
}

// estimateTokens uses a chars/4 heuristic, a commonly cited rough
// approximation for English text under BPE-style tokenizers. It's
// deliberately dependency-free rather than pulling in a real tokenizer.
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	n := len(text) / 4
	if n == 0 {
		n = 1
	}
	return n
}
