// Command local-swarm-mcp runs an MCP server that delegates mechanical
// tasks to local or remote OpenAI-compatible inference backends, and
// provides context-management tools (compaction, a scratch store, and
// token estimation) so an MCP client can manage its own context budget.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/store"
	"github.com/jhonsferg/local-swarm-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "local-swarm-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", defaultConfigPath(), "path to config YAML")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	scratchStore, err := store.Open(cfg.StorePath)
	if err != nil {
		return fmt.Errorf("open scratch store: %w", err)
	}
	defer func() { _ = scratchStore.Close() }()

	registry := backend.NewRegistry(cfg.Backends)
	client := backend.NewClient()

	backendTools := &tools.Backends{Registry: registry}
	delegator := &tools.Delegator{Registry: registry, Client: client}
	compactor := &tools.Compactor{Registry: registry, Client: client}
	scratch := &tools.Scratch{Store: scratchStore}

	mcpServer := server.NewMCPServer("local-swarm-mcp", version)

	mcpServer.AddTool(tools.ListBackendsTool(), backendTools.ListBackendsHandler)
	mcpServer.AddTool(tools.HealthCheckTool(), backendTools.HealthCheckHandler)
	mcpServer.AddTool(tools.DelegateTaskTool(), delegator.DelegateTaskHandler)
	mcpServer.AddTool(tools.CompactContextTool(), compactor.CompactContextHandler)
	mcpServer.AddTool(tools.ScratchSetTool(), scratch.ScratchSetHandler)
	mcpServer.AddTool(tools.ScratchGetTool(), scratch.ScratchGetHandler)
	mcpServer.AddTool(tools.ScratchListTool(), scratch.ScratchListHandler)
	mcpServer.AddTool(tools.ScratchDeleteTool(), scratch.ScratchDeleteHandler)
	mcpServer.AddTool(tools.EstimateTokensTool(), tools.EstimateTokensHandler)
	mcpServer.AddTool(tools.ClassifyTaskRiskTool(), tools.ClassifyTaskRiskHandler)

	return server.ServeStdio(mcpServer)
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "local-swarm-mcp.yaml"
	}
	return filepath.Join(dir, "local-swarm-mcp", "config.yaml")
}
