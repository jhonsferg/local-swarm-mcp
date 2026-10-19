package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// scriptedToolCallServer replies with a tool_calls response on its first
// call, then a final-text response on every call after, simulating a model
// that calls one tool before answering.
func scriptedToolCallServer(t *testing.T) *httptest.Server {
	t.Helper()
	var calls atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "fakeindex__index_repository",
								"arguments": map[string]any{"repo_path": "D:/repo"},
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "indexed successfully"}}},
		})
	}))
}

// alwaysToolCallServer always replies with a tool call, simulating a model
// that never settles on a final answer - used to test max_iterations.
func alwaysToolCallServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant", "content": "",
					"tool_calls": []map[string]any{{
						"id": "call_x", "type": "function",
						"function": map[string]any{"name": "fakeindex__index_repository", "arguments": map[string]any{"repo_path": "D:/repo"}},
					}},
				},
			}},
		})
	}))
}

func fakeIndexManager(t *testing.T) *mcpdownstream.Manager {
	t.Helper()
	s := mcpserver.NewMCPServer("fakeindex", "0.0.1")
	indexTool := mcp.NewTool("index_repository",
		mcp.WithDescription("Index a repository"),
		mcp.WithString("repo_path", mcp.Required()),
	)
	s.AddTool(indexTool, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, _ := req.RequireString("repo_path")
		return mcp.NewToolResultText("indexed " + path + ": 42 files"), nil
	})

	c, err := mcpclient.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}

	mgr, err := mcpdownstream.Connect(t.Context(), nil) // start empty, attach below
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(mgr.Close)

	if err := mgr.Attach(t.Context(), "fakeindex", c); err != nil {
		t.Fatalf("attach: %v", err)
	}
	return mgr
}

func TestAgentTask_CallsToolThenAnswers(t *testing.T) {
	srv := scriptedToolCallServer(t)
	defer srv.Close()
	mgr := fakeIndexManager(t)

	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	id, err := tr.SpawnAgent("", "you are terse", "index D:/repo", mgr, nil, 8, 64, 0.1, 0)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}

	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if info.Status != TaskCompleted {
		t.Fatalf("expected TaskCompleted, got %+v", info)
	}
	if info.Result != "indexed successfully" {
		t.Fatalf("unexpected result: %q", info.Result)
	}
	if len(info.Transcript) != 1 {
		t.Fatalf("expected 1 transcript entry, got %d: %+v", len(info.Transcript), info.Transcript)
	}
	entry := info.Transcript[0]
	if entry.ToolName != "fakeindex__index_repository" || entry.Arguments["repo_path"] != "D:/repo" {
		t.Fatalf("unexpected transcript entry: %+v", entry)
	}
	if entry.Result != "indexed D:/repo: 42 files" {
		t.Fatalf("unexpected tool result in transcript: %q", entry.Result)
	}
}

func TestAgentTask_MaxIterationsGivesUp(t *testing.T) {
	srv := alwaysToolCallServer(t)
	defer srv.Close()
	mgr := fakeIndexManager(t)

	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	id, err := tr.SpawnAgent("", "", "loop forever", mgr, nil, 3, 64, 0.1, 0)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}

	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if info.Status != TaskFailed {
		t.Fatalf("expected TaskFailed after exhausting max_iterations, got %+v", info)
	}
	if len(info.Transcript) != 3 {
		t.Fatalf("expected 3 transcript entries (one per iteration), got %d", len(info.Transcript))
	}
}

func TestAgentTask_UnknownBackend(t *testing.T) {
	mgr := fakeIndexManager(t)
	tr := NewTaskRegistry(backend.NewClient(), backend.NewRegistry(nil))
	if _, err := tr.SpawnAgent("nope", "", "hi", mgr, nil, 8, 64, 0.1, 0); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

// toolCallThenAnswerServer replies with a single tool call to toolName on
// its first call, then a final answer - simulating a model that names a
// tool of its own choosing, whether or not it was offered.
func toolCallThenAnswerServer(t *testing.T, toolName string) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{{
							"id": "call_1", "type": "function",
							"function": map[string]any{"name": toolName, "arguments": map[string]any{}},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "done"}}},
		})
	}))
}

// attachCountingServer attaches a second downstream server, "danger", whose
// single tool records how many times it actually ran.
func attachCountingServer(t *testing.T, mgr *mcpdownstream.Manager) *atomic.Int32 {
	t.Helper()
	var ran atomic.Int32
	s := mcpserver.NewMCPServer("danger", "0.0.1")
	s.AddTool(mcp.NewTool("wipe", mcp.WithDescription("Destructive tool")), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ran.Add(1)
		return mcp.NewToolResultText("wiped"), nil
	})
	c, err := mcpclient.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	if err := mgr.Attach(t.Context(), "danger", c); err != nil {
		t.Fatalf("attach: %v", err)
	}
	return &ran
}

func TestAgentTask_ToolOutsideServerFilterIsRefused(t *testing.T) {
	// Reproduces a confirmed bug: mcp_servers only limited which tools were
	// offered to the model, not which ones it could actually invoke, so a
	// model naming another server's tool ran it anyway.
	srv := toolCallThenAnswerServer(t, "danger__wipe")
	defer srv.Close()
	mgr := fakeIndexManager(t)
	ran := attachCountingServer(t, mgr)

	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	id, err := tr.SpawnAgent("", "", "clean up", mgr, []string{"fakeindex"}, 4, 64, 0.1, 0)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}
	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}

	if ran.Load() != 0 {
		t.Fatalf("danger__wipe ran %d time(s) despite mcp_servers=[fakeindex]", ran.Load())
	}
	if len(info.Transcript) != 1 || !info.Transcript[0].IsError || info.Transcript[0].ToolName != "danger__wipe" {
		t.Fatalf("expected one refused danger__wipe call in the transcript, got %+v", info.Transcript)
	}
}

func TestAgentTask_UnofferedToolIsRefused(t *testing.T) {
	// A tool name the model made up must be refused before reaching any
	// downstream server, even with no server filter at all.
	srv := toolCallThenAnswerServer(t, "fakeindex__delete_repository")
	defer srv.Close()
	mgr := fakeIndexManager(t)

	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	id, err := tr.SpawnAgent("", "", "hi", mgr, nil, 4, 64, 0.1, 0)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}
	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if len(info.Transcript) != 1 || !info.Transcript[0].IsError {
		t.Fatalf("expected one refused call in the transcript, got %+v", info.Transcript)
	}
	if want := `tool "fakeindex__delete_repository" is not available`; !strings.Contains(info.Transcript[0].Result, want) {
		t.Fatalf("transcript result = %q, want it to contain %q", info.Transcript[0].Result, want)
	}
}

func TestAgentTask_ToolInsideServerFilterStillRuns(t *testing.T) {
	srv := toolCallThenAnswerServer(t, "danger__wipe")
	defer srv.Close()
	mgr := fakeIndexManager(t)
	ran := attachCountingServer(t, mgr)

	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	id, err := tr.SpawnAgent("", "", "clean up", mgr, []string{"danger"}, 4, 64, 0.1, 0)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}
	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if ran.Load() != 1 || len(info.Transcript) != 1 || info.Transcript[0].IsError {
		t.Fatalf("expected danger__wipe to run once when allowed, ran=%d transcript=%+v", ran.Load(), info.Transcript)
	}
}
