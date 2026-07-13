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
	"net/http"

	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
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

// Server implements the admin HTTP handlers.
type Server struct {
	Version string
	Hosts   *hostregistry.Registry
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
