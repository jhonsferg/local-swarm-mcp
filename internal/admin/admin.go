// Package admin implements the small HTTP control surface a local
// local-swarm-mcp process uses to detect whether another instance is
// already serving (GET /health) and to ask it to register a new inference
// host on its behalf (POST /register-host) - the mechanism behind
// "--register-host" and the associated MCP tools working without any
// config file edit or process restart.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
	"github.com/jhonsferg/local-swarm-mcp/internal/logging"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpserverregistry"
)

// HealthResponse identifies a running daemon and its version, so a probing
// process can distinguish "a real local-swarm-mcp daemon answered" from
// "something else happens to be listening on this port".
type HealthResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
}

// RegisterHostRequest is the POST /register-host body.
type RegisterHostRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key,omitempty"`
}

// RegisterMCPServerRequest is the POST /mcp-servers body.
type RegisterMCPServerRequest struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// MCPServerStatus is a registered downstream MCP server plus whether it's
// currently connected.
type MCPServerStatus struct {
	mcpserverregistry.Server
	Connected bool `json:"connected"`
}

// Server implements the admin HTTP handlers.
type Server struct {
	Version    string
	Hosts      *hostregistry.Registry
	Backends   *backend.Registry
	MCPServers *mcpserverregistry.Registry
	Downstream *mcpdownstream.Manager
	Logs       *logging.Hub
	// OnRegistered, if set, is called after a host is persisted so the
	// caller can trigger an immediate poll instead of waiting for the next
	// tick.
	OnRegistered func(ctx context.Context, host hostregistry.Host)
}

// Handler returns an http.Handler serving the admin endpoints, to be
// mounted under a prefix (e.g. "/admin/") alongside the MCP endpoint.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/health", s.handleHealth)
	mux.HandleFunc("/admin/register-host", s.handleRegisterHost)
	mux.HandleFunc("/admin/unregister-host", s.handleUnregisterHost)
	mux.HandleFunc("/admin/hosts", s.handleListHosts)
	mux.HandleFunc("/admin/register-mcp-server", s.handleRegisterMCPServer)
	mux.HandleFunc("/admin/unregister-mcp-server", s.handleUnregisterMCPServer)
	mux.HandleFunc("/admin/mcp-servers", s.handleListMCPServers)
	mux.HandleFunc("/admin/backends", s.handleListBackends)
	mux.HandleFunc("/admin/logs", s.handleLogs)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Service: "local-swarm-mcp", Version: s.Version})
}

func (s *Server) handleRegisterHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.BaseURL == "" {
		http.Error(w, "name and base_url are required", http.StatusBadRequest)
		return
	}

	host := hostregistry.Host{Name: req.Name, BaseURL: req.BaseURL, APIKey: req.APIKey}
	if err := s.Hosts.RegisterHost(host); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.OnRegistered != nil {
		s.OnRegistered(r.Context(), host)
	}
	writeJSON(w, http.StatusOK, host)
}

func (s *Server) handleUnregisterHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Hosts.UnregisterHost(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.Hosts.Status())
}

func (s *Server) handleRegisterMCPServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Command == "" {
		http.Error(w, "name and command are required", http.StatusBadRequest)
		return
	}

	srv := mcpserverregistry.Server{Name: req.Name, Command: req.Command, Args: req.Args}
	if err := s.MCPServers.Register(srv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Downstream.ConnectOne(r.Context(), config.MCPServer{Name: req.Name, Command: req.Command, Args: req.Args}); err != nil {
		http.Error(w, "registered but failed to connect: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (s *Server) handleUnregisterMCPServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.MCPServers.Unregister(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.Downstream.Detach(req.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	servers, err := s.MCPServers.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	connected := make(map[string]bool)
	for _, name := range s.Downstream.Names() {
		connected[name] = true
	}
	out := make([]MCPServerStatus, 0, len(servers))
	for _, srv := range servers {
		out = append(out, MCPServerStatus{Server: srv, Connected: connected[srv.Name]})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListBackends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.Backends.List())
}

// handleLogs streams log lines as Server-Sent Events: recent history
// first, then every new line as it's written, until the client
// disconnects.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for _, line := range s.Logs.Recent() {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.TrimRight(line, "\n"))
	}
	flusher.Flush()

	ch, cancel := s.Logs.Subscribe()
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.TrimRight(line, "\n"))
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
