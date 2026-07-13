package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

func TestCompleteWithTools_ParsesToolCalls(t *testing.T) {
	var receivedTools []ToolSpec

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		receivedTools = req.Tools

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "codebase-memory-mcp__index_repository",
									"arguments": map[string]any{"repo_path": "D:/repo"},
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewClient()
	b := config.Backend{Name: "test", BaseURL: srv.URL, Model: "m"}
	tools := []ToolSpec{{
		Type: "function",
		Function: FunctionSpec{
			Name:        "codebase-memory-mcp__index_repository",
			Description: "Index a repo",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"repo_path":{"type":"string"}}}`),
		},
	}}

	result, err := client.CompleteWithTools(t.Context(), b, []ChatMessage{{Role: "user", Content: "hi"}}, tools, 64, 0.1)
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}

	if len(receivedTools) != 1 || receivedTools[0].Function.Name != "codebase-memory-mcp__index_repository" {
		t.Fatalf("tools were not sent to the backend: %+v", receivedTools)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	call := result.ToolCalls[0]
	if call.Function.Name != "codebase-memory-mcp__index_repository" {
		t.Fatalf("unexpected tool call name: %q", call.Function.Name)
	}
	args, err := call.Function.ArgumentsMap()
	if err != nil {
		t.Fatalf("ArgumentsMap: %v", err)
	}
	if args["repo_path"] != "D:/repo" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestComplete_StillWorksWithoutTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Tools != nil {
			t.Errorf("expected no tools field when Complete is used, got %+v", req.Tools)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "PONG"}}},
		})
	}))
	defer srv.Close()

	client := NewClient()
	b := config.Backend{Name: "test", BaseURL: srv.URL, Model: "m"}
	reply, err := client.Complete(t.Context(), b, []ChatMessage{{Role: "user", Content: "ping"}}, 10, 0.1)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply != "PONG" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}
