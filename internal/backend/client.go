package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

// ChatMessage is a single OpenAI-style chat message. ToolCalls is set on an
// assistant message that requested tool invocations; ToolCallID identifies
// which tool call a "tool"-role message is answering.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolSpec describes a tool made available to the model, in the OpenAI
// function-calling shape.
type ToolSpec struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

// FunctionSpec is the "function" body of a ToolSpec.
type FunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction is the "function" body of a ToolCall.
type ToolCallFunction struct {
	Name string `json:"name"`
	// Arguments varies by provider: proper OpenAI sends a JSON-encoded
	// string; Ollama has been observed sending an already-decoded object.
	// json.RawMessage captures either shape as-is; use ArgumentsMap to get
	// a usable map regardless of which one a given backend sent.
	Arguments json.RawMessage `json:"arguments"`
}

// ArgumentsMap decodes Arguments into a map, handling both the
// already-decoded-object shape and the JSON-encoded-string shape different
// providers use.
func (f ToolCallFunction) ArgumentsMap() (map[string]any, error) {
	var asObject map[string]any
	if err := json.Unmarshal(f.Arguments, &asObject); err == nil {
		return asObject, nil
	}

	var asString string
	if err := json.Unmarshal(f.Arguments, &asString); err != nil {
		return nil, fmt.Errorf("arguments is neither a JSON object nor a JSON-encoded string: %w", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(asString), &decoded); err != nil {
		return nil, fmt.Errorf("parse arguments string: %w", err)
	}
	return decoded, nil
}

// CompletionResult is a chat completion's assistant message: either final
// text (ToolCalls empty) or a request to invoke one or more tools (Content
// may be empty in that case).
type CompletionResult struct {
	Content   string
	ToolCalls []ToolCall
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []ToolSpec    `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client is a minimal client for the OpenAI-compatible chat-completions API
// implemented by llama.cpp's llama-server, Ollama, vLLM, and most hosted
// providers.
type Client struct {
	httpClient *http.Client
}

// NewClient builds a Client with a generous timeout suitable for local
// inference on modest hardware.
func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 120 * time.Second}}
}

// Complete sends messages to the given backend and returns the assistant's
// reply text. Equivalent to CompleteWithTools with no tools.
func (c *Client) Complete(ctx context.Context, b config.Backend, messages []ChatMessage, maxTokens int, temperature float64) (string, error) {
	result, err := c.CompleteWithTools(ctx, b, messages, nil, maxTokens, temperature)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// CompleteWithTools sends messages and an optional set of tool
// declarations to the given backend, returning either final text or the
// tool calls the model wants to make. Passing a model that doesn't support
// structured tool-calling is not an error: it will simply come back with
// ToolCalls empty and its attempt (if any) folded into Content as plain
// text - callers that care should verify their chosen model actually
// produces ToolCalls before relying on this path.
func (c *Client) CompleteWithTools(ctx context.Context, b config.Backend, messages []ChatMessage, tools []ToolSpec, maxTokens int, temperature float64) (CompletionResult, error) {
	reqBody := chatRequest{
		Model:       b.Model,
		Messages:    messages,
		Tools:       tools,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResult{}, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(b.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return CompletionResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CompletionResult{}, fmt.Errorf("request backend %q: %w", b.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResult{}, fmt.Errorf("read response from backend %q: %w", b.Name, err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return CompletionResult{}, fmt.Errorf("parse response from backend %q (status %d): %w", b.Name, resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return CompletionResult{}, fmt.Errorf("backend %q returned an error: %s", b.Name, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return CompletionResult{}, fmt.Errorf("backend %q returned status %d: %s", b.Name, resp.StatusCode, string(body))
	}
	if len(parsed.Choices) == 0 {
		return CompletionResult{}, fmt.Errorf("backend %q returned no choices", b.Name)
	}

	msg := parsed.Choices[0].Message
	return CompletionResult{Content: msg.Content, ToolCalls: msg.ToolCalls}, nil
}
