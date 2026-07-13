package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jhonsferg/local-swarm-mcp/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
)

// Scratch bundles the store dependency the scratch_* tools need.
type Scratch struct {
	Store *store.Store
}

// ScratchSetTool returns the MCP tool definition for scratch_set.
func ScratchSetTool() mcp.Tool {
	return mcp.NewTool("scratch_set",
		mcp.WithDescription("Store a value under a key in a persistent local scratch space, outside Claude's own context. Use to stash large intermediate results you only need to reference, not re-read in full."),
		mcp.WithString("key", mcp.Required(), mcp.Description("Key to store under")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Value to store")),
	)
}

// ScratchSetHandler stores a key/value pair.
func (s *Scratch) ScratchSetHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	value, err := req.RequireString("value")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.Store.Set(key, value); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("stored %q (%d bytes)", key, len(value))), nil
}

// ScratchGetTool returns the MCP tool definition for scratch_get.
func ScratchGetTool() mcp.Tool {
	return mcp.NewTool("scratch_get",
		mcp.WithDescription("Retrieve a value previously stored with scratch_set."),
		mcp.WithString("key", mcp.Required(), mcp.Description("Key to retrieve")),
	)
}

// ScratchGetHandler retrieves a value by key.
func (s *Scratch) ScratchGetHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	value, ok, err := s.Store.Get(key)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("no value stored under key %q", key)), nil
	}
	return mcp.NewToolResultText(value), nil
}

// ScratchListTool returns the MCP tool definition for scratch_list.
func ScratchListTool() mcp.Tool {
	return mcp.NewTool("scratch_list",
		mcp.WithDescription("List all keys currently stored in the scratch space."),
	)
}

// ScratchListHandler lists all stored keys.
func (s *Scratch) ScratchListHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	keys, err := s.Store.List()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out, err := json.Marshal(keys)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

// ScratchDeleteTool returns the MCP tool definition for scratch_delete.
func ScratchDeleteTool() mcp.Tool {
	return mcp.NewTool("scratch_delete",
		mcp.WithDescription("Delete a key from the scratch space."),
		mcp.WithString("key", mcp.Required(), mcp.Description("Key to delete")),
	)
}

// ScratchDeleteHandler deletes a key.
func (s *Scratch) ScratchDeleteHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.Store.Delete(key); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("deleted %q", key)), nil
}
