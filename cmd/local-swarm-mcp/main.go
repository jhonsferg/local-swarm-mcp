// Command local-swarm-mcp runs an MCP server that delegates mechanical
// tasks to local or remote OpenAI-compatible inference backends, and
// provides context-management tools (compaction, a scratch store, task
// orchestration, and multi-turn sessions) so an MCP client can manage its
// own context budget and treat delegated work like its own agents.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/admin"
	"github.com/jhonsferg/local-swarm-mcp/internal/authmw"
	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/daemon"
	"github.com/jhonsferg/local-swarm-mcp/internal/discovery"
	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
	"github.com/jhonsferg/local-swarm-mcp/internal/logging"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpserverregistry"
	"github.com/jhonsferg/local-swarm-mcp/internal/orchestrator"
	"github.com/jhonsferg/local-swarm-mcp/internal/store"
	"github.com/jhonsferg/local-swarm-mcp/internal/tools"
	"github.com/jhonsferg/local-swarm-mcp/internal/webui"
	"github.com/mark3labs/mcp-go/server"
	bolt "go.etcd.io/bbolt"
)

// version is overridden at build time via -ldflags "-X main.version=vX.Y.Z"
// (goreleaser does this for release binaries); a plain `go build` keeps it
// as "dev".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "local-swarm-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = usage
	showVersion := flag.Bool("version", false, "print the version and exit")
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
	hostStorePathFlag := flag.String("host-store-path", "", "override the discovered-hosts database file path")
	pollInterval := flag.Duration("poll-interval", discovery.DefaultInterval, "how often to poll registered hosts for models")
	registerHost := flag.Bool("register-host", false, `register a new inference host for background model discovery and exit (or, if no daemon is running yet, become it) - requires -name and -host-base-url`)
	hostName := flag.String("name", "", "host name for -register-host, e.g. \"rx9070\" or \"dgx-spark\"")
	hostBaseURL := flag.String("host-base-url", "", "Ollama root URL for -register-host, e.g. http://192.168.18.29:11434 (no /v1 suffix)")
	hostAPIKey := flag.String("host-api-key", "", "API key for the host being registered via -register-host, if any")
	mcpServerStorePathFlag := flag.String("mcp-server-store-path", "", "override the registered-downstream-MCP-servers database file path")
	logPathFlag := flag.String("log-path", "", "override the daemon log file location")
	ui := flag.Bool("ui", false, "serve the embedded dashboard (backends, hosts, downstream servers, live logs) at / - only meaningful for -transport=http")
	flag.Parse()

	if *showVersion {
		fmt.Println("local-swarm-mcp", version)
		return nil
	}

	if *registerHost {
		if *hostName == "" || *hostBaseURL == "" {
			return fmt.Errorf("-register-host requires -name and -host-base-url")
		}
	}

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
	// Zero backends at startup is fine for every transport: register one
	// afterward via -register-host, the dashboard, or an MCP tool call
	// (wireDynamicToolsForStdio below gives -transport stdio the same
	// register_backend_host/register_downstream_mcp_server tools the
	// daemon has, backed by the same persisted store), all with no
	// restart needed.

	scratchStore, err := store.Open(cfg.StorePath)
	if err != nil {
		return fmt.Errorf("open scratch store: %w", err)
	}
	defer func() { _ = scratchStore.Close() }()

	registry := backend.NewRegistry(cfg.Backends)
	client := backend.NewClient()
	taskRegistry := orchestrator.NewTaskRegistry(client, registry)
	sessionRegistry := orchestrator.NewSessionRegistry(client, registry)

	downstream, err := mcpdownstream.Connect(context.Background(), cfg.MCPServers)
	if err != nil {
		// Non-fatal: a misconfigured downstream server shouldn't prevent
		// the rest of the server (delegate_task, sessions, scratch store,
		// any downstream server that DID connect) from working. Agent
		// tasks pointed at the failed server will simply find no tools.
		fmt.Fprintln(os.Stderr, "local-swarm-mcp: warning:", err)
	}
	defer downstream.Close()

	mcpServer := server.NewMCPServer("local-swarm-mcp", version)
	registerTools(mcpServer, registry, client, scratchStore, taskRegistry, sessionRegistry, downstream)

	daemonCfg := daemonConfig{
		addr:               *httpAddr,
		apiKey:             *apiKey,
		insecureNoAuth:     *insecureNoAuth,
		hostStorePath:      hostStorePath(*hostStorePathFlag),
		mcpServerStorePath: mcpServerStorePath(*mcpServerStorePathFlag),
		logPath:            logPath(*logPathFlag),
		pollInterval:       *pollInterval,
		ui:                 *ui,
	}

	if *registerHost {
		daemonCfg.pendingHost = &hostregistry.Host{Name: *hostName, BaseURL: *hostBaseURL, APIKey: *hostAPIKey}
		return serveDaemon(mcpServer, registry, downstream, daemonCfg)
	}

	switch *transport {
	case "stdio":
		cleanup := wireDynamicToolsForStdio(mcpServer, registry, downstream, daemonCfg)
		defer cleanup()
		return server.ServeStdio(mcpServer)
	case "http":
		return serveDaemon(mcpServer, registry, downstream, daemonCfg)
	default:
		return fmt.Errorf("unknown -transport %q (want \"stdio\" or \"http\")", *transport)
	}
}

// wireDynamicToolsForStdio gives a -transport stdio session the same
// register_backend_host/unregister_backend_host/list_backend_hosts and
// register_downstream_mcp_server/unregister/list tools the HTTP daemon
// has, backed by the same persisted stores (cfg.hostStorePath /
// cfg.mcpServerStorePath) and a background poller - so a bare `local-swarm-mcp`
// invocation (no daemon, no flags) can still add inference hardware and
// downstream MCP servers progressively, through the MCP connection itself,
// exactly like the daemon's tools/dashboard/-register-host CLI do.
//
// This is opportunistic, not exclusive: unlike the daemon (which goes
// through leader election first), a stdio session doesn't try to become
// the one true owner of these stores - it just tries to open them with a
// short timeout, and quietly does without these tools if something else
// (typically an already-running -transport http daemon) holds the lock,
// rather than delaying MCP startup or erroring out. The MCP client still
// gets every other tool either way; only host/MCP-server management is
// affected, and only when a store is already exclusively held elsewhere.
func wireDynamicToolsForStdio(
	mcpServer *server.MCPServer,
	registry *backend.Registry,
	downstream *mcpdownstream.Manager,
	cfg daemonConfig,
) (cleanup func()) {
	noop := func() {}

	const lockTimeout = 500 * time.Millisecond
	openOpts := &bolt.Options{Timeout: lockTimeout}

	hostReg, err := hostregistry.OpenWithOptions(cfg.hostStorePath, openOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local-swarm-mcp: host discovery unavailable this session (store busy, likely held by a running -transport http daemon):", err)
		return noop
	}

	mcpServerReg, err := mcpserverregistry.OpenWithOptions(cfg.mcpServerStorePath, openOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local-swarm-mcp: downstream MCP server management unavailable this session (store busy, likely held by a running -transport http daemon):", err)
		_ = hostReg.Close()
		return noop
	}

	ctx, cancel := context.WithCancel(context.Background())
	reconnectPersistedServers(ctx, mcpServerReg, downstream, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	poller := discovery.NewPoller(hostReg, registry)
	poller.Interval = cfg.pollInterval
	go poller.Run(ctx)

	triggerPoll := func(ctx context.Context, host hostregistry.Host) { poller.PollOnce(ctx, host) }

	hostTools := &tools.Hosts{Registry: hostReg, OnRegistered: triggerPoll}
	mcpServer.AddTool(tools.RegisterBackendHostTool(), hostTools.RegisterBackendHostHandler)
	mcpServer.AddTool(tools.UnregisterBackendHostTool(), hostTools.UnregisterBackendHostHandler)
	mcpServer.AddTool(tools.ListBackendHostsTool(), hostTools.ListBackendHostsHandler)

	mcpServerTools := &tools.MCPServers{Registry: mcpServerReg, Downstream: downstream}
	mcpServer.AddTool(tools.RegisterDownstreamMCPServerTool(), mcpServerTools.RegisterDownstreamMCPServerHandler)
	mcpServer.AddTool(tools.UnregisterDownstreamMCPServerTool(), mcpServerTools.UnregisterDownstreamMCPServerHandler)
	mcpServer.AddTool(tools.ListDownstreamMCPServersTool(), mcpServerTools.ListDownstreamMCPServersHandler)

	return func() {
		cancel()
		_ = hostReg.Close()
		_ = mcpServerReg.Close()
	}
}

// daemonConfig bundles serveDaemon's parameters - grouped into a struct
// once it grew past a handful of positional args across the host
// discovery and downstream-MCP-server discovery features.
type daemonConfig struct {
	addr               string
	apiKey             string
	insecureNoAuth     bool
	hostStorePath      string
	mcpServerStorePath string
	logPath            string
	pollInterval       time.Duration
	ui                 bool
	pendingHost        *hostregistry.Host
}

// serveDaemon is the entry point for every HTTP-transport invocation
// (a plain "-transport http" start, or "-register-host"). Exactly one
// process becomes the persistent daemon (serving MCP + admin, and
// optionally the dashboard, over addr); any other concurrently-started
// process detects the healthy daemon and either exits cleanly (plain
// start) or forwards its pending host registration to it over HTTP
// (register-host) instead. See internal/daemon for the election mechanics.
func serveDaemon(
	mcpServer *server.MCPServer,
	registry *backend.Registry,
	downstream *mcpdownstream.Manager,
	cfg daemonConfig,
) error {
	healthCheck := func() error { return checkDaemonHealth(cfg.addr, cfg.apiKey) }

	role, lock, err := daemon.Elect(defaultLockPath(), healthCheck)
	if err != nil {
		return err
	}

	if role == daemon.RoleClient {
		if cfg.pendingHost == nil {
			fmt.Fprintf(os.Stderr, "local-swarm-mcp: a daemon is already running and healthy at %s - nothing to do\n", cfg.addr)
			return nil
		}
		return forwardRegisterHost(cfg.addr, cfg.apiKey, *cfg.pendingHost)
	}
	defer func() { _ = lock.Release() }()

	logHub := logging.NewHub(1000)
	logger, closeLog, err := logging.New(cfg.logPath, logHub)
	if err != nil {
		return err
	}
	defer func() { _ = closeLog() }()
	logger.Info("daemon starting", "addr", cfg.addr, "ui", cfg.ui)

	// A generous but bounded timeout: normally nothing else holds these
	// files, so this opens immediately either way, but a stdio session
	// started earlier may be opportunistically sharing the same store
	// (see wireDynamicToolsForStdio) - fail with a clear error after a
	// few seconds instead of hanging forever waiting for it to close.
	storeOpenOpts := &bolt.Options{Timeout: 5 * time.Second}

	hostReg, err := hostregistry.OpenWithOptions(cfg.hostStorePath, storeOpenOpts)
	if err != nil {
		return err
	}
	defer func() { _ = hostReg.Close() }()

	mcpServerReg, err := mcpserverregistry.OpenWithOptions(cfg.mcpServerStorePath, storeOpenOpts)
	if err != nil {
		return err
	}
	defer func() { _ = mcpServerReg.Close() }()
	reconnectPersistedServers(context.Background(), mcpServerReg, downstream, logger)

	poller := discovery.NewPoller(hostReg, registry)
	poller.Interval = cfg.pollInterval

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go poller.Run(ctx)

	if cfg.pendingHost != nil {
		if err := hostReg.RegisterHost(*cfg.pendingHost); err != nil {
			return fmt.Errorf("register host %q: %w", cfg.pendingHost.Name, err)
		}
		logger.Info("registered host, becoming the daemon", "host", cfg.pendingHost.Name, "addr", cfg.addr)
	}

	triggerPoll := func(ctx context.Context, host hostregistry.Host) {
		logger.Info("polling newly registered host", "host", host.Name)
		poller.PollOnce(ctx, host)
	}

	hostTools := &tools.Hosts{Registry: hostReg, OnRegistered: triggerPoll}
	mcpServer.AddTool(tools.RegisterBackendHostTool(), hostTools.RegisterBackendHostHandler)
	mcpServer.AddTool(tools.UnregisterBackendHostTool(), hostTools.UnregisterBackendHostHandler)
	mcpServer.AddTool(tools.ListBackendHostsTool(), hostTools.ListBackendHostsHandler)

	mcpServerTools := &tools.MCPServers{Registry: mcpServerReg, Downstream: downstream}
	mcpServer.AddTool(tools.RegisterDownstreamMCPServerTool(), mcpServerTools.RegisterDownstreamMCPServerHandler)
	mcpServer.AddTool(tools.UnregisterDownstreamMCPServerTool(), mcpServerTools.UnregisterDownstreamMCPServerHandler)
	mcpServer.AddTool(tools.ListDownstreamMCPServersTool(), mcpServerTools.ListDownstreamMCPServersHandler)

	adminServer := &admin.Server{
		Version:      version,
		Hosts:        hostReg,
		Backends:     registry,
		MCPServers:   mcpServerReg,
		Downstream:   downstream,
		Logs:         logHub,
		OnRegistered: triggerPoll,
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(mcpServer))
	mux.Handle("/admin/", adminServer.Handler())

	if cfg.ui {
		uiHandler, err := webui.Handler()
		if err != nil {
			return fmt.Errorf("build embedded dashboard: %w", err)
		}
		mux.Handle("/", uiHandler)
		logger.Info("dashboard enabled", "url", "http://"+displayAddr(cfg.addr)+"/")
	}

	return serveHTTP(mux, cfg.addr, cfg.apiKey, cfg.insecureNoAuth)
}

// reconnectPersistedServers connects every downstream MCP server
// registered in a previous run (or by a separate register-downstream-mcp
// call before this process won the daemon election), so it survives a
// daemon restart without needing to be re-registered.
func reconnectPersistedServers(ctx context.Context, reg *mcpserverregistry.Registry, downstream *mcpdownstream.Manager, logger *slog.Logger) {
	servers, err := reg.List()
	if err != nil {
		logger.Error("failed to load persisted downstream MCP servers", "error", err)
		return
	}
	for _, srv := range servers {
		if err := downstream.ConnectOne(ctx, config.MCPServer{Name: srv.Name, Command: srv.Command, Args: srv.Args}); err != nil {
			logger.Warn("failed to reconnect persisted downstream MCP server", "server", srv.Name, "error", err)
			continue
		}
		logger.Info("reconnected persisted downstream MCP server", "server", srv.Name)
	}
}

// displayAddr renders a ":port"-style listen address as something
// browsable on localhost.
func displayAddr(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return "localhost" + addr
	}
	return addr
}

// checkDaemonHealth confirms a process is answering as a real
// local-swarm-mcp daemon at addr, not just something else on that port.
func checkDaemonHealth(addr, apiKey string) error {
	url := "http://" + addr + "/admin/health"
	if len(addr) > 0 && addr[0] == ':' {
		url = "http://localhost" + addr + "/admin/health"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var health admin.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return fmt.Errorf("decode health response: %w", err)
	}
	if health.Service != "local-swarm-mcp" {
		return fmt.Errorf("unexpected service identity %q", health.Service)
	}
	return nil
}

// forwardRegisterHost sends a pending host registration to the daemon
// already running at addr, used when -register-host lost the election.
func forwardRegisterHost(addr, apiKey string, host hostregistry.Host) error {
	url := "http://" + addr + "/admin/register-host"
	if len(addr) > 0 && addr[0] == ':' {
		url = "http://localhost" + addr + "/admin/register-host"
	}
	body, err := json.Marshal(admin.RegisterHostRequest{Name: host.Name, BaseURL: host.BaseURL, APIKey: host.APIKey})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send registration to running daemon at %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("running daemon rejected registration (status %d)", resp.StatusCode)
	}
	fmt.Fprintf(os.Stderr, "local-swarm-mcp: registered host %q with the running daemon at %s\n", host.Name, addr)
	return nil
}

func hostStorePath(override string) string {
	if override != "" {
		return override
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "local-swarm-mcp-hosts.db"
	}
	return filepath.Join(dir, "local-swarm-mcp", "hosts.db")
}

func mcpServerStorePath(override string) string {
	if override != "" {
		return override
	}
	return mcpserverregistry.DefaultPath()
}

func logPath(override string) string {
	if override != "" {
		return override
	}
	return logging.DefaultLogPath()
}

func defaultLockPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "local-swarm-mcp.lock"
	}
	return filepath.Join(dir, "local-swarm-mcp", "daemon.lock")
}

// serveHTTP runs the MCP server over Streamable HTTP so it can be reached
// across the network - e.g. hosted on a separate GPU machine (a DGX Spark,
// a desktop with a discrete GPU, etc.) rather than spawned as a local
// stdio subprocess. Requires -api-key unless -insecure-no-auth explicitly
// accepts an unauthenticated listener (only reasonable on a trusted,
// isolated network).
func serveHTTP(handler http.Handler, addr, apiKey string, insecureNoAuth bool) error {
	if apiKey == "" && !insecureNoAuth {
		return fmt.Errorf("-transport=http requires -api-key (or explicit -insecure-no-auth for a trusted, isolated network)")
	}

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "local-swarm-mcp: WARNING - serving HTTP with -insecure-no-auth, no bearer token required")
		return http.ListenAndServe(addr, handler) //nolint:gosec // timeouts aren't meaningful for a long-lived MCP streaming endpoint
	}

	authed := authmw.RequireBearer(handler, apiKey)
	fmt.Fprintf(os.Stderr, "local-swarm-mcp: serving HTTP on %s (bearer token required, MCP at /mcp, admin at /admin)\n", addr)
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
	downstream *mcpdownstream.Manager,
) {
	backendTools := &tools.Backends{Registry: registry}
	delegator := &tools.Delegator{Registry: registry, Client: client}
	compactor := &tools.Compactor{Registry: registry, Client: client}
	scratch := &tools.Scratch{Store: scratchStore}
	taskTools := &tools.Tasks{Registry: taskRegistry}
	sessionTools := &tools.Sessions{Registry: sessionRegistry}
	agentTools := &tools.Agents{TaskRegistry: taskRegistry, Downstream: downstream}

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
	s.AddTool(tools.SpawnAgentTaskTool(), agentTools.SpawnAgentTaskHandler)
	s.AddTool(tools.ListAvailableAgentToolsTool(), agentTools.ListAvailableAgentToolsHandler)
}

// usage replaces the default flag-package help output with flags grouped
// by concern, plus a couple of runnable examples mirroring the README.
func usage() {
	fmt.Fprintf(os.Stderr, `local-swarm-mcp %s

An MCP server that delegates mechanical tasks to local or remote
OpenAI-compatible inference backends, and lets an MCP client spawn
background tasks, multi-turn sessions, and tool-using agents against them.

Usage:
  local-swarm-mcp [flags]

Config (legacy - see "Dashboard" and "Host discovery" below for the
primary, config-file-free way to run local-swarm-mcp now):
  -config string
        path to config file, YAML or JSON (default "%s")
  -config-format string
        force "yaml" or "json" parsing (default: auto-detect from -config's extension)

Ad-hoc backend (added on top of any config-file backends; entirely
optional - zero backends configured at startup is fine, add one later via
register_backend_host, -register-host, or the dashboard):
  -backend-name string
        name for the ad-hoc backend (default "cli")
  -backend-url string
        base URL, e.g. http://localhost:8080/v1
  -backend-model string
        model name
  -backend-key string
        API key, if any

Transport:
  -transport string
        "stdio" (spawned as a local subprocess) or "http" (a persistent daemon) (default "stdio")
  -http-addr string
        listen address when -transport=http (default ":8090")
  -api-key string
        bearer token HTTP clients must present when -transport=http
  -insecure-no-auth
        allow -transport=http with no -api-key (trusted, isolated networks only)

Storage:
  -store-path string
        override the scratch-store file location
  -host-store-path string
        override the discovered-hosts database file location
  -mcp-server-store-path string
        override the registered-downstream-MCP-servers database file location
  -log-path string
        override the daemon log file location
  -poll-interval duration
        how often to poll registered hosts for models (default 30s)

Host discovery (register an inference host without a config file edit or
restart - see -register-host below; only meaningful for -transport=http,
since discovery needs the persistent daemon to poll in the background):
  -register-host
        register a host and exit (or, if no daemon is running yet at
        -http-addr, become it) - requires -name and -host-base-url
  -name string
        host name for -register-host, e.g. "rx9070" or "dgx-spark"
  -host-base-url string
        Ollama root URL for -register-host, e.g. http://192.168.18.29:11434
        (no /v1 suffix)
  -host-api-key string
        API key for the host being registered via -register-host, if any

Dashboard:
  -ui
        serve the embedded dashboard (backends, hosts, downstream MCP
        servers, live logs) at / - only meaningful for -transport=http

Other:
  -version
        print the version and exit
  -h, -help
        show this help

A YAML/JSON -config file still works, but it's the legacy path: the
persistent daemon (-transport http) is the primary way to configure
local-swarm-mcp now. Backends come from -register-host + background
discovery instead of a static "backends:" list, and downstream MCP servers
(e.g. codebase-memory-mcp) come from the register_downstream_mcp_server
MCP tool or POST /admin/register-mcp-server instead of a static
"mcp_servers:" list - both take effect immediately, no restart needed.

Examples:
  # The primary path: a persistent daemon with the dashboard, no config file
  local-swarm-mcp -transport http -insecure-no-auth -ui

  # No config file - a single ad-hoc backend from flags alone
  local-swarm-mcp -backend-url http://localhost:8080/v1 -backend-model qwen2.5-coder

  # Hosted on a separate GPU machine, reachable over HTTP
  local-swarm-mcp -transport http -http-addr 0.0.0.0:8090 -api-key <token> -ui

  # Register a newly-arrived host (e.g. a DGX Spark just joined the LAN) with
  # whatever daemon is already running at the default address - no config
  # file edit, no restart; if no daemon is running yet, this becomes it
  local-swarm-mcp -register-host -name dgx-spark -host-base-url http://192.168.1.50:11434

See https://github.com/jhonsferg/local-swarm-mcp for the full tool reference and config format.
`, version, defaultConfigPath())
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "local-swarm-mcp.yaml"
	}
	return filepath.Join(dir, "local-swarm-mcp", "config.yaml")
}
