package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolCallRecord captures one tool invocation an agent task made, for
// auditing what it actually did rather than just trusting its final answer.
type ToolCallRecord struct {
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    string         `json:"result"`
	IsError   bool           `json:"is_error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// DefaultMaxIterations bounds an agent task's tool-call loop when the
// caller doesn't specify one, so a model that never settles on a final
// answer can't run forever.
const DefaultMaxIterations = 8

// SpawnAgent starts a tool-using agent task in the background: the model
// is given the tools discovered from mgr (filtered to serverFilter, or all
// of them if empty) and may call them in a loop, up to maxIterations,
// before producing a final answer. Returns the task ID immediately -
// tracked with the same TaskInfo/Status/Wait/Cancel/List machinery as a
// plain Spawn task.
func (tr *TaskRegistry) SpawnAgent(
	backendName, systemPrompt, prompt string,
	mgr *mcpdownstream.Manager,
	serverFilter []string,
	maxIterations, maxTokens int,
	temperature, topP float64,
) (string, error) {
	cfg, err := tr.registry.Get(backendName)
	if err != nil {
		return "", err
	}
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}

	id := newID()
	ctx, cancel := context.WithCancel(context.Background())
	t := &task{id: id, backend: cfg.Name, status: TaskPending, startedAt: time.Now(), cancel: cancel}

	tr.mu.Lock()
	tr.tasks[id] = t
	tr.mu.Unlock()

	go tr.runAgent(ctx, t, systemPrompt, prompt, mgr, serverFilter, maxIterations, maxTokens, temperature, topP)

	return id, nil
}

func (tr *TaskRegistry) runAgent(
	ctx context.Context,
	t *task,
	systemPrompt, prompt string,
	mgr *mcpdownstream.Manager,
	serverFilter []string,
	maxIterations, maxTokens int,
	temperature, topP float64,
) {
	tr.mu.Lock()
	t.status = TaskRunning
	tr.mu.Unlock()

	cfg, err := tr.registry.Get(t.backend)
	if err != nil {
		tr.finish(t, "", nil, err, ctx)
		return
	}

	toolSpecs, err := buildToolSpecs(mgr, serverFilter)
	if err != nil {
		tr.finish(t, "", nil, err, ctx)
		return
	}

	var messages []backend.ChatMessage
	if systemPrompt != "" {
		messages = append(messages, backend.ChatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, backend.ChatMessage{Role: "user", Content: prompt})

	var transcript []ToolCallRecord

	for i := 0; i < maxIterations; i++ {
		result, err := tr.client.CompleteWithTools(ctx, cfg, messages, toolSpecs, maxTokens, temperature, topP)
		if err != nil {
			tr.finish(t, "", transcript, err, ctx)
			return
		}

		if len(result.ToolCalls) == 0 {
			tr.finish(t, result.Content, transcript, nil, ctx)
			return
		}

		messages = append(messages, backend.ChatMessage{
			Role:      "assistant",
			Content:   result.Content,
			ToolCalls: result.ToolCalls,
		})

		for _, call := range result.ToolCalls {
			record, toolMessage := tr.invokeTool(ctx, mgr, call)
			transcript = append(transcript, record)
			messages = append(messages, toolMessage)
		}
	}

	tr.finish(t, "", transcript, fmt.Errorf("reached max_iterations (%d) without a final answer", maxIterations), ctx)
}

// invokeTool calls a single tool the model requested and builds both the
// audit record and the "tool"-role message to feed back to the model.
func (tr *TaskRegistry) invokeTool(ctx context.Context, mgr *mcpdownstream.Manager, call backend.ToolCall) (ToolCallRecord, backend.ChatMessage) {
	args, err := call.Function.ArgumentsMap()
	if err != nil {
		record := ToolCallRecord{ToolName: call.Function.Name, Result: fmt.Sprintf("error parsing arguments: %v", err), IsError: true, Timestamp: time.Now()}
		return record, backend.ChatMessage{Role: "tool", Content: record.Result, ToolCallID: call.ID}
	}

	callResult, err := mgr.CallTool(ctx, call.Function.Name, args)
	if err != nil {
		record := ToolCallRecord{ToolName: call.Function.Name, Arguments: args, Result: fmt.Sprintf("error calling tool: %v", err), IsError: true, Timestamp: time.Now()}
		return record, backend.ChatMessage{Role: "tool", Content: record.Result, ToolCallID: call.ID}
	}

	text := extractText(callResult)
	record := ToolCallRecord{ToolName: call.Function.Name, Arguments: args, Result: text, IsError: callResult.IsError, Timestamp: time.Now()}
	return record, backend.ChatMessage{Role: "tool", Content: text, ToolCallID: call.ID}
}

// extractText concatenates a CallToolResult's text content blocks.
func extractText(result *mcp.CallToolResult) string {
	var out string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out
}

// buildToolSpecs converts every discovered downstream tool (filtered to
// serverFilter, or all of them if empty) into the OpenAI function-calling
// shape the backend client sends to the model.
func buildToolSpecs(mgr *mcpdownstream.Manager, serverFilter []string) ([]backend.ToolSpec, error) {
	tools := mgr.ListAllTools(serverFilter)
	specs := make([]backend.ToolSpec, 0, len(tools))
	for _, t := range tools {
		schema, err := json.Marshal(t.Tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal input schema for %q: %w", t.QualifiedName, err)
		}
		specs = append(specs, backend.ToolSpec{
			Type: "function",
			Function: backend.FunctionSpec{
				Name:        t.QualifiedName,
				Description: t.Tool.Description,
				Parameters:  schema,
			},
		})
	}
	return specs, nil
}
