package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/orchestrator"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tasks bundles the task-registry dependency the spawn/status/wait/list/cancel
// tools need.
type Tasks struct {
	Registry *orchestrator.TaskRegistry
}

// SpawnTaskTool returns the MCP tool definition for spawn_task.
func SpawnTaskTool() mcp.Tool {
	return mcp.NewTool("spawn_task",
		mcp.WithDescription("Start a task on a backend in the background and return immediately with a task ID. Use for longer-running delegated work you don't want to block on; poll with task_status or block with wait_task. Mirrors spawning a background agent."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The task/prompt to send")),
		mcp.WithString("backend", mcp.Description("Backend name to use; omit to use the first configured backend")),
		mcp.WithString("system_prompt", mcp.Description("Optional system prompt")),
		mcp.WithNumber("max_tokens", mcp.DefaultNumber(1024), mcp.Description("Maximum tokens to generate")),
		mcp.WithNumber("temperature", mcp.DefaultNumber(0.2), mcp.Description("Sampling temperature")),
		mcp.WithNumber("top_p", mcp.Description("Nucleus sampling threshold (0-1); omit to use the backend's own default")),
	)
}

// SpawnTaskHandler starts a background task and returns its ID.
func (t *Tasks) SpawnTaskHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt, err := req.RequireString("prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	backendName := req.GetString("backend", "")
	systemPrompt := req.GetString("system_prompt", "")
	maxTokens := int(req.GetFloat("max_tokens", 1024))
	temperature := req.GetFloat("temperature", 0.2)
	topP := req.GetFloat("top_p", 0)

	id, err := t.Registry.Spawn(backendName, systemPrompt, prompt, maxTokens, temperature, topP)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(id), nil
}

// TaskStatusTool returns the MCP tool definition for task_status.
func TaskStatusTool() mcp.Tool {
	return mcp.NewTool("task_status",
		mcp.WithDescription("Non-blocking snapshot of a spawned task's state (pending/running/completed/failed/cancelled), including its result once completed."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID returned by spawn_task")),
	)
}

// TaskStatusHandler returns a task's current snapshot as JSON.
func (t *Tasks) TaskStatusHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	info, err := t.Registry.Status(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(info)
}

// WaitTaskTool returns the MCP tool definition for wait_task.
func WaitTaskTool() mcp.Tool {
	return mcp.NewTool("wait_task",
		mcp.WithDescription("Block until a spawned task finishes (or timeout_seconds elapses), then return its final snapshot. Use when you need the result before continuing, without the manual poll loop task_status requires."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID returned by spawn_task")),
		mcp.WithNumber("timeout_seconds", mcp.DefaultNumber(120), mcp.Description("Maximum time to wait")),
	)
}

// WaitTaskHandler blocks until the task finishes or times out.
func (t *Tasks) WaitTaskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeout := time.Duration(req.GetFloat("timeout_seconds", 120)) * time.Second

	info, err := t.Registry.Wait(ctx, id, timeout)
	if err != nil {
		// Still return whatever snapshot we have alongside the timeout/cancel error.
		out, marshalErr := json.Marshal(info)
		if marshalErr == nil {
			return mcp.NewToolResultError(err.Error() + " snapshot=" + string(out)), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(info)
}

// ListTasksTool returns the MCP tool definition for list_tasks.
func ListTasksTool() mcp.Tool {
	return mcp.NewTool("list_tasks",
		mcp.WithDescription("List every task spawned this server run, with their current status."),
	)
}

// ListTasksHandler returns all tracked tasks.
func (t *Tasks) ListTasksHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return jsonResult(t.Registry.List())
}

// CancelTaskTool returns the MCP tool definition for cancel_task.
func CancelTaskTool() mcp.Tool {
	return mcp.NewTool("cancel_task",
		mcp.WithDescription("Cancel a still-running task."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID returned by spawn_task")),
	)
}

// CancelTaskHandler cancels a task in progress.
func (t *Tasks) CancelTaskHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := t.Registry.Cancel(id); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("cancelled"), nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}
