package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func callClassify(t *testing.T, description string) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"description": description}

	result, err := ClassifyTaskRiskHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("ClassifyTaskRiskHandler: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty result content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}

func TestClassifyTaskRisk(t *testing.T) {
	if got := callClassify(t, "generate boilerplate getters for this struct"); got[:9] != "risk=low:" {
		t.Errorf("expected low risk, got %q", got)
	}
	if got := callClassify(t, "force-push this branch to main"); got[:10] != "risk=high:" {
		t.Errorf("expected high risk, got %q", got)
	}
	if got := callClassify(t, "rotate the API key credential"); got[:10] != "risk=high:" {
		t.Errorf("expected high risk, got %q", got)
	}
}
