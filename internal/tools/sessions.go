package tools

import (
	"context"

	"github.com/jhonsferg/local-swarm-mcp/internal/orchestrator"
	"github.com/mark3labs/mcp-go/mcp"
)

// Sessions bundles the session-registry dependency the session tools need.
type Sessions struct {
	Registry *orchestrator.SessionRegistry
}

// CreateSessionTool returns the MCP tool definition for create_session.
func CreateSessionTool() mcp.Tool {
	return mcp.NewTool("create_session",
		mcp.WithDescription("Open a persistent multi-turn conversation against a backend. Use for iterative delegated work where later messages need earlier context, instead of re-sending the full history yourself each call."),
		mcp.WithString("backend", mcp.Description("Backend name to use; omit to use the first configured backend")),
		mcp.WithString("system_prompt", mcp.Description("Optional system prompt for the whole session")),
	)
}

// CreateSessionHandler opens a new session and returns its ID.
func (s *Sessions) CreateSessionHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	backendName := req.GetString("backend", "")
	systemPrompt := req.GetString("system_prompt", "")
	id, err := s.Registry.Create(backendName, systemPrompt)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(id), nil
}

// SendMessageTool returns the MCP tool definition for send_message.
func SendMessageTool() mcp.Tool {
	return mcp.NewTool("send_message",
		mcp.WithDescription("Send a message to an open session, carrying its full prior history, and return the reply."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("Session ID returned by create_session")),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message to send")),
		mcp.WithNumber("max_tokens", mcp.DefaultNumber(1024), mcp.Description("Maximum tokens to generate")),
		mcp.WithNumber("temperature", mcp.DefaultNumber(0.2), mcp.Description("Sampling temperature")),
	)
}

// SendMessageHandler sends a message within an existing session.
func (s *Sessions) SendMessageHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	message, err := req.RequireString("message")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	maxTokens := int(req.GetFloat("max_tokens", 1024))
	temperature := req.GetFloat("temperature", 0.2)

	reply, err := s.Registry.Send(ctx, id, message, maxTokens, temperature)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(reply), nil
}

// SessionHistoryTool returns the MCP tool definition for session_history.
func SessionHistoryTool() mcp.Tool {
	return mcp.NewTool("session_history",
		mcp.WithDescription("Return the full message history of a session."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("Session ID returned by create_session")),
	)
}

// SessionHistoryHandler returns a session's message history as JSON.
func (s *Sessions) SessionHistoryHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	history, err := s.Registry.History(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(history)
}

// CloseSessionTool returns the MCP tool definition for close_session.
func CloseSessionTool() mcp.Tool {
	return mcp.NewTool("close_session",
		mcp.WithDescription("Discard a session and its history."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("Session ID returned by create_session")),
	)
}

// CloseSessionHandler closes a session.
func (s *Sessions) CloseSessionHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.Registry.Close(id); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("closed"), nil
}

// ListSessionsTool returns the MCP tool definition for list_sessions.
func ListSessionsTool() mcp.Tool {
	return mcp.NewTool("list_sessions",
		mcp.WithDescription("List every open session, with its backend and message count."),
	)
}

// ListSessionsHandler returns all open sessions.
func (s *Sessions) ListSessionsHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return jsonResult(s.Registry.List())
}
