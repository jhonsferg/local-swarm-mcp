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

// ChatMessage is a single OpenAI-style chat message.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
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
// reply text.
func (c *Client) Complete(ctx context.Context, b config.Backend, messages []ChatMessage, maxTokens int, temperature float64) (string, error) {
	reqBody := chatRequest{
		Model:       b.Model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(b.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request backend %q: %w", b.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response from backend %q: %w", b.Name, err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse response from backend %q (status %d): %w", b.Name, resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("backend %q returned an error: %s", b.Name, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("backend %q returned status %d: %s", b.Name, resp.StatusCode, string(body))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("backend %q returned no choices", b.Name)
	}
	return parsed.Choices[0].Message.Content, nil
}
