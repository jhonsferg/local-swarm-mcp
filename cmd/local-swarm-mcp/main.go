// Command local-swarm-mcp runs an MCP server that delegates mechanical
// tasks to local or remote OpenAI-compatible inference backends, and
// provides context-management tools (compaction, a scratch store, task
// orchestration, and multi-turn sessions) so an MCP client can manage its
// own context budget and treat delegated work like its own agents.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jhonsferg/local-swarm-mcp/internal/authmw"
	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/orchestrator"
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
	configPath := flag.String("config", defaultConfigPath(), "path to config file (YAML or JSON)")
	configFormat := flag.String("config-format", "", `config file format: "yaml" or "json" (default: auto-detect from the -config extension)`)
	backendName := flag.String("backend-name", "cli", "name for an ad-hoc backend given via -backend-url (added on top of any config file backends)")
	backendURL := flag.String("backend-url", "", "base URL for an ad-hoc OpenAI-compatible backend, e.g. http://localhost:8080/v1")
	backendModel := flag.String("backend-model", "", "model name for the ad-hoc backend")
	backendKey := flag.String("backend-key", "", "API key for the ad-hoc backend, if any")
	storePathFlag := flag.String("store-path", "", "override the scratch-store file path")
	transport := flag.String("transport", "stdio", `how the MCP client reaches this server: "stdio" (spawned as a local subprocess) or "http" (a standalone network service, e.g. running on a remote GPU machine)`)
	httpAddr := flag.String("http-addr", ":8090", `listen address when -transport=http, e.g. ":8090" or "0.0.0.0:8090"`)
	apiKey := flag.String("api-key", "", "bearer token required of HTTP clients when -transport=http; required unless -insecure-no-auth is set")
	insecureNoAuth := flag.Bool("insecure-no-auth", false, "allow -transport=http with no -api-key (only for a trusted, isolated network)")
	flag.Parse()

	cfg, err := loadConfig(*configPath, *configFormat)
	if err != nil {
		return err
	}

	if *backendURL != "" {
		cfg.Backends = append(cfg.Backends, config.Backend{
			Name:    *backendName,
			BaseURL: *backendURL,
			Model:   *backendModel,
			APIKey:  *backendKey,
		})
	}
	if *storePathFlag != "" {
		cfg.StorePath = *storePathFlag
	}
	if cfg.StorePath == "" {
		cfg.StorePath = config.DefaultStorePath()
	}
	if len(cfg.Backends) == 0 {
		return fmt.Errorf("no backends configured: provide -config pointing at a config file, or -backend-url/-backend-model for an ad-hoc backend")
	}

	scratchStore, err := store.Open(cfg.StorePath)
	if err != nil {
		return fmt.Errorf("open scratch store: %w", err)
	}
	defer func() { _ = scratchStore.Close() }()

	registry := backend.NewRegistry(cfg.Backends)
	client := backend.NewClient()
	taskRegistry := orchestrator.NewTaskRegistry(client, registry)
	sessionRegistry := orchestrator.NewSessionRegistry(client, registry)

	mcpServer := server.NewMCPServer("local-swarm-mcp", version)
	registerTools(mcpServer, registry, client, scratchStore, taskRegistry, sessionRegistry)

	switch *transport {
	case "stdio":
		return server.ServeStdio(mcpServer)
	case "http":
		return serveHTTP(mcpServer, *httpAddr, *apiKey, *insecureNoAuth)
	default:
		return fmt.Errorf("unknown -transport %q (want \"stdio\" or \"http\")", *transport)
	}
}

// serveHTTP runs the MCP server over Streamable HTTP so it can be reached
// across the network - e.g. hosted on a separate GPU machine (a DGX Spark,
// a desktop with a discrete GPU, etc.) rather than spawned as a local
// stdio subprocess. Requires -api-key unless -insecure-no-auth explicitly
// accepts an unauthenticated listener (only reasonable on a trusted,
// isolated network).
func serveHTTP(mcpServer *server.MCPServer, addr, apiKey string, insecureNoAuth bool) error {
	if apiKey == "" && !insecureNoAuth {
		return fmt.Errorf("-transport=http requires -api-key (or explicit -insecure-no-auth for a trusted, isolated network)")
	}

	httpServer := server.NewStreamableHTTPServer(mcpServer)

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "local-swarm-mcp: WARNING - serving HTTP with -insecure-no-auth, no bearer token required")
		return httpServer.Start(addr)
	}

	authed := authmw.RequireBearer(httpServer, apiKey)
	fmt.Fprintf(os.Stderr, "local-swarm-mcp: serving HTTP on %s (bearer token required)\n", addr)
	return http.ListenAndServe(addr, authed) //nolint:gosec // timeouts aren't meaningful for a long-lived MCP streaming endpoint
}

// loadConfig reads the config file at path if it exists, or returns an
// empty Config if it doesn't - a missing config file is not an error on
// its own, since a backend can be supplied entirely via -backend-* flags.
func loadConfig(path, format string) (*config.Config, error) {
	if _, err := os.Stat(path); err != nil {
		return &config.Config{}, nil
	}
	cfg, err := config.Load(path, format)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func registerTools(
	s *server.MCPServer,
	registry *backend.Registry,
	client *backend.Client,
	scratchStore *store.Store,
	taskRegistry *orchestrator.TaskRegistry,
	sessionRegistry *orchestrator.SessionRegistry,
) {
	backendTools := &tools.Backends{Registry: registry}
	delegator := &tools.Delegator{Registry: registry, Client: client}
	compactor := &tools.Compactor{Registry: registry, Client: client}
	scratch := &tools.Scratch{Store: scratchStore}
	taskTools := &tools.Tasks{Registry: taskRegistry}
	sessionTools := &tools.Sessions{Registry: sessionRegistry}

	s.AddTool(tools.ListBackendsTool(), backendTools.ListBackendsHandler)
	s.AddTool(tools.HealthCheckTool(), backendTools.HealthCheckHandler)
	s.AddTool(tools.DelegateTaskTool(), delegator.DelegateTaskHandler)
	s.AddTool(tools.CompactContextTool(), compactor.CompactContextHandler)
	s.AddTool(tools.ScratchSetTool(), scratch.ScratchSetHandler)
	s.AddTool(tools.ScratchGetTool(), scratch.ScratchGetHandler)
	s.AddTool(tools.ScratchListTool(), scratch.ScratchListHandler)
	s.AddTool(tools.ScratchDeleteTool(), scratch.ScratchDeleteHandler)
	s.AddTool(tools.EstimateTokensTool(), tools.EstimateTokensHandler)
	s.AddTool(tools.ClassifyTaskRiskTool(), tools.ClassifyTaskRiskHandler)
	s.AddTool(tools.SpawnTaskTool(), taskTools.SpawnTaskHandler)
	s.AddTool(tools.TaskStatusTool(), taskTools.TaskStatusHandler)
	s.AddTool(tools.WaitTaskTool(), taskTools.WaitTaskHandler)
	s.AddTool(tools.ListTasksTool(), taskTools.ListTasksHandler)
	s.AddTool(tools.CancelTaskTool(), taskTools.CancelTaskHandler)
	s.AddTool(tools.CreateSessionTool(), sessionTools.CreateSessionHandler)
	s.AddTool(tools.SendMessageTool(), sessionTools.SendMessageHandler)
	s.AddTool(tools.SessionHistoryTool(), sessionTools.SessionHistoryHandler)
	s.AddTool(tools.CloseSessionTool(), sessionTools.CloseSessionHandler)
	s.AddTool(tools.ListSessionsTool(), sessionTools.ListSessionsHandler)
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "local-swarm-mcp.yaml"
	}
	return filepath.Join(dir, "local-swarm-mcp", "config.yaml")
}
