// Package orchestrator gives an MCP client full control over delegated work
// running on local-swarm-mcp's backends: fire-and-forget background tasks
// (spawn/status/wait/cancel/list) and persistent multi-turn sessions
// (create/send/history/close/list) - the same primitives an agent
// orchestration system offers for its own subagents, backed by a local
// model instead.
package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
)

// TaskStatus is the lifecycle state of a spawned task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

type task struct {
	id         string
	backend    string
	status     TaskStatus
	result     string
	errMsg     string
	startedAt  time.Time
	finishedAt time.Time
	cancel     context.CancelFunc
	// transcript is set only for agent tasks (SpawnAgent): every tool call
	// the model made, its arguments, and the result, so a caller can audit
	// what an agent actually did rather than just trust its final answer.
	transcript []ToolCallRecord
}

// TaskInfo is a safe, immutable snapshot of a task's state for callers.
type TaskInfo struct {
	ID         string           `json:"id"`
	Backend    string           `json:"backend"`
	Status     TaskStatus       `json:"status"`
	Result     string           `json:"result,omitempty"`
	Error      string           `json:"error,omitempty"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	Transcript []ToolCallRecord `json:"transcript,omitempty"`
}

// TaskRegistry tracks background tasks spawned against a backend.
type TaskRegistry struct {
	mu       sync.Mutex
	tasks    map[string]*task
	client   *backend.Client
	registry *backend.Registry
}

// NewTaskRegistry builds an empty TaskRegistry.
func NewTaskRegistry(client *backend.Client, registry *backend.Registry) *TaskRegistry {
	return &TaskRegistry{
		tasks:    make(map[string]*task),
		client:   client,
		registry: registry,
	}
}

// Spawn starts a task against backendName (or the default backend, if
// empty) in the background and returns its ID immediately.
func (tr *TaskRegistry) Spawn(backendName, systemPrompt, prompt string, maxTokens int, temperature float64) (string, error) {
	cfg, err := tr.registry.Get(backendName)
	if err != nil {
		return "", err
	}

	id := newID()
	ctx, cancel := context.WithCancel(context.Background())
	t := &task{id: id, backend: cfg.Name, status: TaskPending, startedAt: time.Now(), cancel: cancel}

	tr.mu.Lock()
	tr.tasks[id] = t
	tr.mu.Unlock()

	go tr.run(ctx, t, systemPrompt, prompt, maxTokens, temperature)

	return id, nil
}

func (tr *TaskRegistry) run(ctx context.Context, t *task, systemPrompt, prompt string, maxTokens int, temperature float64) {
	tr.mu.Lock()
	t.status = TaskRunning
	tr.mu.Unlock()

	cfg, err := tr.registry.Get(t.backend)
	if err != nil {
		tr.finish(t, "", nil, err, ctx)
		return
	}

	var messages []backend.ChatMessage
	if systemPrompt != "" {
		messages = append(messages, backend.ChatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, backend.ChatMessage{Role: "user", Content: prompt})

	result, err := tr.client.Complete(ctx, cfg, messages, maxTokens, temperature)
	tr.finish(t, result, nil, err, ctx)
}

// finish records a task's terminal state atomically: result/error status
// and (for agent tasks) its tool-call transcript together, so a concurrent
// Status()/List() call never observes one without the other.
func (tr *TaskRegistry) finish(t *task, result string, transcript []ToolCallRecord, err error, ctx context.Context) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	t.finishedAt = time.Now()
	t.transcript = transcript
	switch {
	case err != nil && ctx.Err() != nil:
		t.status = TaskCancelled
	case err != nil:
		t.status = TaskFailed
		t.errMsg = err.Error()
	default:
		t.status = TaskCompleted
		t.result = result
	}
}

func toInfo(t *task) TaskInfo {
	info := TaskInfo{
		ID:         t.id,
		Backend:    t.backend,
		Status:     t.status,
		Result:     t.result,
		Error:      t.errMsg,
		StartedAt:  t.startedAt,
		Transcript: t.transcript,
	}
	if !t.finishedAt.IsZero() {
		fin := t.finishedAt
		info.FinishedAt = &fin
	}
	return info
}

// Status returns a snapshot of the given task's current state.
func (tr *TaskRegistry) Status(id string) (TaskInfo, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	t, ok := tr.tasks[id]
	if !ok {
		return TaskInfo{}, fmt.Errorf("unknown task %q", id)
	}
	return toInfo(t), nil
}

// List returns a snapshot of every tracked task.
func (tr *TaskRegistry) List() []TaskInfo {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]TaskInfo, 0, len(tr.tasks))
	for _, t := range tr.tasks {
		out = append(out, toInfo(t))
	}
	return out
}

// Cancel requests cancellation of a still-running task.
func (tr *TaskRegistry) Cancel(id string) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	t, ok := tr.tasks[id]
	if !ok {
		return fmt.Errorf("unknown task %q", id)
	}
	if t.status != TaskPending && t.status != TaskRunning {
		return fmt.Errorf("task %q already finished (status=%s)", id, t.status)
	}
	t.cancel()
	return nil
}

// Wait blocks (bounded by timeout, and by ctx) until the task leaves the
// pending/running states, then returns its final snapshot.
func (tr *TaskRegistry) Wait(ctx context.Context, id string, timeout time.Duration) (TaskInfo, error) {
	deadline := time.Now().Add(timeout)
	for {
		info, err := tr.Status(id)
		if err != nil {
			return TaskInfo{}, err
		}
		if info.Status != TaskPending && info.Status != TaskRunning {
			return info, nil
		}
		if time.Now().After(deadline) {
			return info, fmt.Errorf("timed out after %s waiting for task %q (status=%s)", timeout, id, info.Status)
		}
		select {
		case <-ctx.Done():
			return info, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
