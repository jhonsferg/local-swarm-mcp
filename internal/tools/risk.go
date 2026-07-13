package tools

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type riskPattern struct {
	phrase string
	reason string
}

// riskyPatterns is a deliberately simple, fast, rule-based check - not a
// model call. It flags tasks whose description mentions destructive
// operations, secrets, or judgment-heavy work, so Claude (or the user) can
// decide those need Claude's own attention rather than delegation.
var riskyPatterns = []riskPattern{
	{"force push", "force-push is a destructive, hard-to-reverse git operation"},
	{"force-push", "force-push is a destructive, hard-to-reverse git operation"},
	{"rm -rf", "recursive force delete"},
	{"drop table", "destructive database operation"},
	{"delete branch", "branch deletion needs explicit authorization"},
	{"reset --hard", "discards uncommitted work"},
	{"credential", "touches secrets/credentials"},
	{"password", "touches secrets/credentials"},
	{"api key", "touches secrets/credentials"},
	{"secret", "touches secrets/credentials"},
	{"production", "targets a production environment"},
	{"architecture", "architectural decision - needs judgment, not mechanical execution"},
	{"security", "security-sensitive - needs careful review"},
}

// ClassifyTaskRiskTool returns the MCP tool definition for classify_task_risk.
func ClassifyTaskRiskTool() mcp.Tool {
	return mcp.NewTool("classify_task_risk",
		mcp.WithDescription("Fast rule-based check (no model call) on a task description, flagging whether it looks safe to delegate to a local model vs. needing Claude's own judgment. Not authoritative - a clean result doesn't guarantee safety, only that no obvious red flag matched."),
		mcp.WithString("description", mcp.Required(), mcp.Description("Description of the task under consideration")),
	)
}

// ClassifyTaskRiskHandler scans description for known risky phrases.
func ClassifyTaskRiskHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	description, err := req.RequireString("description")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	lower := strings.ToLower(description)
	var reasons []string
	for _, p := range riskyPatterns {
		if strings.Contains(lower, p.phrase) {
			reasons = append(reasons, p.reason)
		}
	}

	if len(reasons) == 0 {
		return mcp.NewToolResultText("risk=low: no known red flags matched; still use judgment"), nil
	}
	return mcp.NewToolResultText("risk=high: " + strings.Join(reasons, "; ")), nil
}
