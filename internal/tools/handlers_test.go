package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/orchestrator"
	"github.com/jhonsferg/local-swarm-mcp/internal/store"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// chatServer is a fake OpenAI-compatible backend that answers every
// completion with reply and records the last request body.
func chatServer(t *testing.T, reply string, last *map[string]any) *backend.Registry {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if last != nil {
			_ = json.NewDecoder(r.Body).Decode(last)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}},
		})
	}))
	t.Cleanup(srv.Close)
	return backend.NewRegistry([]config.Backend{{Name: "fake", BaseURL: srv.URL, Model: "m", APIKey: "backend-secret"}})
}

func call(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// mustText returns a checker for a handler's (result, err) pair that fails
// the test on an error and returns the result text.
func mustText(t *testing.T) func(*mcp.CallToolResult, error) string {
	return func(result *mcp.CallToolResult, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		text := toolText(t, result)
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", text)
		}
		return text
	}
}

func TestDelegateTaskHandler(t *testing.T) {
	var last map[string]any
	d := &Delegator{Registry: chatServer(t, "done", &last), Client: backend.NewClient()}

	text := mustText(t)(d.DelegateTaskHandler(context.Background(), call(map[string]any{"prompt": "hi", "system_prompt": "terse", "temperature": 0.0})))
	if text != "done" {
		t.Fatalf("reply = %q", text)
	}
	msgs, _ := last["messages"].([]any)
	if len(msgs) != 2 || last["temperature"] != float64(0) {
		t.Fatalf("request = %v, want system+user messages and temperature 0", last)
	}

	res, err := d.DelegateTaskHandler(context.Background(), call(map[string]any{}))
	if err != nil || !res.IsError {
		t.Fatalf("missing prompt should be a tool error, got %v / %v", res, err)
	}
	res, _ = d.DelegateTaskHandler(context.Background(), call(map[string]any{"prompt": "hi", "backend": "nope"}))
	if !res.IsError {
		t.Fatal("unknown backend should be a tool error")
	}
}

func TestCompactContextHandler(t *testing.T) {
	var last map[string]any
	c := &Compactor{Registry: chatServer(t, "short", &last), Client: backend.NewClient()}

	text := mustText(t)(c.CompactContextHandler(context.Background(), call(map[string]any{"text": "long text", "target_tokens": 100, "preserve_instructions": "keep paths"})))
	if text != "short" {
		t.Fatalf("reply = %q", text)
	}
	if last["max_tokens"] != float64(200) {
		t.Fatalf("max_tokens = %v, want 2x target_tokens", last["max_tokens"])
	}
	msgs, _ := last["messages"].([]any)
	user, _ := msgs[1].(map[string]any)
	if content, _ := user["content"].(string); !strings.Contains(content, "keep paths") || !strings.Contains(content, "long text") {
		t.Fatalf("user prompt = %q, want the text and preserve instructions", content)
	}
}

func TestListBackendsHandler_RedactsKeys(t *testing.T) {
	b := &Backends{Registry: chatServer(t, "", nil)}
	text := mustText(t)(b.ListBackendsHandler(context.Background(), call(nil)))
	if strings.Contains(text, "backend-secret") || !strings.Contains(text, `"has_api_key":true`) {
		t.Fatalf("list_backends = %s", text)
	}
}

func TestTaskHandlers_SpawnWaitStatusListCancel(t *testing.T) {
	registry := chatServer(t, "task reply", nil)
	tasks := &Tasks{Registry: orchestrator.NewTaskRegistry(backend.NewClient(), registry)}

	id := mustText(t)(tasks.SpawnTaskHandler(context.Background(), call(map[string]any{"prompt": "hi"})))

	var info orchestrator.TaskInfo
	if err := json.Unmarshal([]byte(mustText(t)(tasks.WaitTaskHandler(context.Background(), call(map[string]any{"task_id": id, "timeout_seconds": 5})))), &info); err != nil {
		t.Fatalf("decode wait_task: %v", err)
	}
	if info.Status != orchestrator.TaskCompleted || info.Result != "task reply" {
		t.Fatalf("wait_task = %+v", info)
	}

	if status := mustText(t)(tasks.TaskStatusHandler(context.Background(), call(map[string]any{"task_id": id}))); !strings.Contains(status, "completed") {
		t.Fatalf("task_status = %s", status)
	}
	if list := mustText(t)(tasks.ListTasksHandler(context.Background(), call(nil))); !strings.Contains(list, id) {
		t.Fatalf("list_tasks = %s, want %s", list, id)
	}
	if res, _ := tasks.CancelTaskHandler(context.Background(), call(map[string]any{"task_id": id})); !res.IsError {
		t.Fatal("cancelling a finished task should be a tool error")
	}
	if res, _ := tasks.TaskStatusHandler(context.Background(), call(map[string]any{"task_id": "nope"})); !res.IsError {
		t.Fatal("unknown task should be a tool error")
	}
}

func TestWaitTaskHandler_TimeoutReturnsSnapshot(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	registry := backend.NewRegistry([]config.Backend{{Name: "slow", BaseURL: slow.URL, Model: "m"}})
	tasks := &Tasks{Registry: orchestrator.NewTaskRegistry(backend.NewClient(), registry)}

	id := mustText(t)(tasks.SpawnTaskHandler(context.Background(), call(map[string]any{"prompt": "hi"})))
	res, _ := tasks.WaitTaskHandler(context.Background(), call(map[string]any{"task_id": id, "timeout_seconds": 0.2}))
	if !res.IsError || !strings.Contains(toolText(t, res), "snapshot=") {
		t.Fatalf("wait_task timeout = %+v, want a tool error carrying the snapshot", res)
	}
	if text := mustText(t)(tasks.CancelTaskHandler(context.Background(), call(map[string]any{"task_id": id}))); text != "cancelled" {
		t.Fatalf("cancel_task = %q", text)
	}
}

func TestSessionHandlers(t *testing.T) {
	sessions := &Sessions{Registry: orchestrator.NewSessionRegistry(backend.NewClient(), chatServer(t, "session reply", nil))}

	id := mustText(t)(sessions.CreateSessionHandler(context.Background(), call(map[string]any{"system_prompt": "terse"})))
	if reply := mustText(t)(sessions.SendMessageHandler(context.Background(), call(map[string]any{"session_id": id, "message": "hi"}))); reply != "session reply" {
		t.Fatalf("send_message = %q", reply)
	}
	if history := mustText(t)(sessions.SessionHistoryHandler(context.Background(), call(map[string]any{"session_id": id}))); !strings.Contains(history, "session reply") {
		t.Fatalf("session_history = %s", history)
	}
	if list := mustText(t)(sessions.ListSessionsHandler(context.Background(), call(nil))); !strings.Contains(list, id) {
		t.Fatalf("list_sessions = %s", list)
	}
	mustText(t)(sessions.CloseSessionHandler(context.Background(), call(map[string]any{"session_id": id})))
	if res, _ := sessions.SendMessageHandler(context.Background(), call(map[string]any{"session_id": id, "message": "hi"})); !res.IsError {
		t.Fatal("send_message on a closed session should be a tool error")
	}
}

func TestScratchHandlers_LazyStore(t *testing.T) {
	lazy := store.NewLazy(filepath.Join(t.TempDir(), "scratch.db"), 100*time.Millisecond)
	t.Cleanup(func() { _ = lazy.Close() })
	s := &Scratch{Lazy: lazy}

	mustText(t)(s.ScratchSetHandler(context.Background(), call(map[string]any{"key": "k", "value": "v"})))
	if v := mustText(t)(s.ScratchGetHandler(context.Background(), call(map[string]any{"key": "k"}))); v != "v" {
		t.Fatalf("scratch_get = %q", v)
	}
	if keys := mustText(t)(s.ScratchListHandler(context.Background(), call(nil))); keys != `["k"]` {
		t.Fatalf("scratch_list = %s", keys)
	}
	mustText(t)(s.ScratchDeleteHandler(context.Background(), call(map[string]any{"key": "k"})))
	if res, _ := s.ScratchGetHandler(context.Background(), call(map[string]any{"key": "k"})); !res.IsError {
		t.Fatal("scratch_get of a deleted key should be a tool error")
	}
}

func newAgentTools(t *testing.T, registry *backend.Registry) *Agents {
	t.Helper()
	s := server.NewMCPServer("fakeindex", "0.0.1")
	s.AddTool(mcp.NewTool("index_repository", mcp.WithDescription("Index a repository")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("indexed"), nil
	})
	c, err := mcpclient.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	mgr, err := mcpdownstream.Connect(t.Context(), nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(mgr.Close)
	if err := mgr.Attach(t.Context(), "fakeindex", c); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return &Agents{TaskRegistry: orchestrator.NewTaskRegistry(backend.NewClient(), registry), Downstream: mgr}
}

func TestListAvailableAgentToolsHandler(t *testing.T) {
	a := newAgentTools(t, backend.NewRegistry(nil))
	text := mustText(t)(a.ListAvailableAgentToolsHandler(context.Background(), call(nil)))
	if !strings.Contains(text, "fakeindex__index_repository") {
		t.Fatalf("list_available_agent_tools = %s", text)
	}
	if text := mustText(t)(a.ListAvailableAgentToolsHandler(context.Background(), call(map[string]any{"mcp_servers": []any{"other"}}))); text != "[]" {
		t.Fatalf("filtered listing = %s, want []", text)
	}
}

func TestSpawnAgentTaskHandler(t *testing.T) {
	a := newAgentTools(t, chatServer(t, "agent answer", nil))

	id := mustText(t)(a.SpawnAgentTaskHandler(context.Background(), call(map[string]any{"prompt": "index it", "mcp_servers": []any{"fakeindex"}})))
	info, err := a.TaskRegistry.Wait(context.Background(), id, 5*time.Second)
	if err != nil || info.Status != orchestrator.TaskCompleted || info.Result != "agent answer" {
		t.Fatalf("agent task = %+v, %v", info, err)
	}

	if res, _ := a.SpawnAgentTaskHandler(context.Background(), call(map[string]any{})); !res.IsError {
		t.Fatal("missing prompt should be a tool error")
	}
	if res, _ := a.SpawnAgentTaskHandler(context.Background(), call(map[string]any{"prompt": "x", "backend": "nope"})); !res.IsError {
		t.Fatal("unknown backend should be a tool error")
	}
}

func TestEstimateTokensAndClassifyRiskHandlers(t *testing.T) {
	if text := mustText(t)(EstimateTokensHandler(context.Background(), call(map[string]any{"text": "12345678"}))); !strings.HasPrefix(text, "~2 tokens") {
		t.Fatalf("estimate_tokens = %q", text)
	}
	if text := mustText(t)(ClassifyTaskRiskHandler(context.Background(), call(map[string]any{"description": "force push to main"}))); !strings.HasPrefix(text, "risk=high") {
		t.Fatalf("classify_task_risk = %q", text)
	}
}
