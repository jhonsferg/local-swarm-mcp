package admin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
	"github.com/jhonsferg/local-swarm-mcp/internal/logging"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpdownstream"
	"github.com/jhonsferg/local-swarm-mcp/internal/mcpserverregistry"
)

func openTestMCPServers(t *testing.T) *mcpserverregistry.Registry {
	t.Helper()
	r, err := mcpserverregistry.Open(filepath.Join(t.TempDir(), "mcp-servers.db"))
	if err != nil {
		t.Fatalf("mcpserverregistry.Open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func newTestDownstream(t *testing.T) *mcpdownstream.Manager {
	t.Helper()
	mgr, err := mcpdownstream.Connect(context.Background(), nil)
	if err != nil {
		t.Fatalf("mcpdownstream.Connect: %v", err)
	}
	t.Cleanup(mgr.Close)
	return mgr
}

func openTestHosts(t *testing.T) *hostregistry.Registry {
	t.Helper()
	r, err := hostregistry.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatalf("hostregistry.Open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestHandleHealth(t *testing.T) {
	s := &Server{Version: "v0.4.0-test", Hosts: openTestHosts(t)}
	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Service != "local-swarm-mcp" || got.Version != "v0.4.0-test" {
		t.Fatalf("got %+v", got)
	}
}

func TestHandleRegisterHost_TriggersOnRegistered(t *testing.T) {
	var calledWith hostregistry.Host
	called := false
	s := &Server{
		Version: "dev",
		Hosts:   openTestHosts(t),
		OnRegistered: func(_ context.Context, h hostregistry.Host) {
			called = true
			calledWith = h
		},
	}

	body, _ := json.Marshal(RegisterHostRequest{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"})
	req := httptest.NewRequest(http.MethodPost, "/admin/register-host", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatalf("OnRegistered was not called")
	}
	if calledWith.Name != "rx9070" {
		t.Fatalf("OnRegistered called with %+v", calledWith)
	}

	hosts, err := s.Hosts.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "rx9070" {
		t.Fatalf("Hosts() = %+v", hosts)
	}
}

func TestHandleRegisterHost_RequiresNameAndBaseURL(t *testing.T) {
	s := &Server{Version: "dev", Hosts: openTestHosts(t)}
	body, _ := json.Marshal(RegisterHostRequest{Name: "rx9070"})
	req := httptest.NewRequest(http.MethodPost, "/admin/register-host", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleListHosts(t *testing.T) {
	hosts := openTestHosts(t)
	if err := hosts.RegisterHost(hostregistry.Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	s := &Server{Version: "dev", Hosts: hosts}

	req := httptest.NewRequest(http.MethodGet, "/admin/hosts", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []hostregistry.HostView
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "rx9070" {
		t.Fatalf("got %+v", got)
	}
}

func TestHandleUnregisterHost(t *testing.T) {
	hosts := openTestHosts(t)
	if err := hosts.RegisterHost(hostregistry.Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	s := &Server{Version: "dev", Hosts: hosts}

	body, _ := json.Marshal(map[string]string{"name": "rx9070"})
	req := httptest.NewRequest(http.MethodPost, "/admin/unregister-host", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	remaining, err := hosts.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("Hosts() after unregister = %+v", remaining)
	}
}

func TestHandleRegisterMCPServer_RequiresNameAndCommand(t *testing.T) {
	s := &Server{Version: "dev", Hosts: openTestHosts(t), MCPServers: openTestMCPServers(t), Downstream: newTestDownstream(t), AllowCommandRegistration: true}
	body, _ := json.Marshal(RegisterMCPServerRequest{Name: "codebase-memory-mcp"})
	req := httptest.NewRequest(http.MethodPost, "/admin/register-mcp-server", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleRegisterMCPServer_PersistsEvenIfConnectFails(t *testing.T) {
	mcpServers := openTestMCPServers(t)
	s := &Server{Version: "dev", Hosts: openTestHosts(t), MCPServers: mcpServers, Downstream: newTestDownstream(t), AllowCommandRegistration: true}

	body, _ := json.Marshal(RegisterMCPServerRequest{Name: "nonexistent", Command: "this-binary-does-not-exist-xyz"})
	req := httptest.NewRequest(http.MethodPost, "/admin/register-mcp-server", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (connect should fail for a nonexistent binary)", rec.Code)
	}

	servers, err := mcpServers.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "nonexistent" {
		t.Fatalf("List() = %+v, want the definition persisted despite the connect failure", servers)
	}
}

func TestHandleUnregisterMCPServer(t *testing.T) {
	mcpServers := openTestMCPServers(t)
	if err := mcpServers.Register(mcpserverregistry.Server{Name: "codebase-memory-mcp", Command: "cmd"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s := &Server{Version: "dev", Hosts: openTestHosts(t), MCPServers: mcpServers, Downstream: newTestDownstream(t), AllowCommandRegistration: true}

	body, _ := json.Marshal(map[string]string{"name": "codebase-memory-mcp"})
	req := httptest.NewRequest(http.MethodPost, "/admin/unregister-mcp-server", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	remaining, err := mcpServers.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("List() after unregister = %+v", remaining)
	}
}

func TestHandleListMCPServers(t *testing.T) {
	mcpServers := openTestMCPServers(t)
	if err := mcpServers.Register(mcpserverregistry.Server{Name: "codebase-memory-mcp", Command: "cmd"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s := &Server{Version: "dev", Hosts: openTestHosts(t), MCPServers: mcpServers, Downstream: newTestDownstream(t), AllowCommandRegistration: true}

	req := httptest.NewRequest(http.MethodGet, "/admin/mcp-servers", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []MCPServerStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "codebase-memory-mcp" || got[0].Connected {
		t.Fatalf("got %+v", got)
	}
}

func TestHandleListBackends(t *testing.T) {
	backends := backend.NewRegistry([]config.Backend{{Name: "llama", Model: "llama3.1:8b"}})
	s := &Server{Version: "dev", Hosts: openTestHosts(t), Backends: backends}

	req := httptest.NewRequest(http.MethodGet, "/admin/backends", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []config.Backend
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "llama" {
		t.Fatalf("got %+v", got)
	}
}

func TestHandleLogs_StreamsRecentThenLive(t *testing.T) {
	hub := logging.NewHub(10)
	_, _ = hub.Write([]byte("past line"))

	s := &Server{Version: "dev", Hosts: openTestHosts(t), Logs: hub}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/admin/logs", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	// Give the handler a moment to flush recent history, then write a new
	// line and cancel the request to end the stream.
	time.Sleep(50 * time.Millisecond)
	_, _ = hub.Write([]byte("live line"))
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleLogs did not exit after request context cancellation")
	}

	body := rec.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}

	if len(events) < 1 || events[0] != "past line" {
		t.Fatalf("events = %+v, want the first to be the pre-existing history line", events)
	}
}

func TestHandleUnregisterHost_UsesUnregisterHostHook(t *testing.T) {
	hosts := openTestHosts(t)
	if err := hosts.RegisterHost(hostregistry.Host{Name: "rx9070", BaseURL: "http://192.168.18.29:11434"}); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}
	var got string
	s := &Server{Version: "dev", Hosts: hosts, UnregisterHost: func(name string) error {
		got = name
		return hosts.UnregisterHost(name)
	}}

	body, _ := json.Marshal(map[string]string{"name": "rx9070"})
	req := httptest.NewRequest(http.MethodPost, "/admin/unregister-host", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got != "rx9070" {
		t.Fatalf("UnregisterHost hook called with %q, want rx9070", got)
	}
}

func TestHandler_RejectsNonJSONPost(t *testing.T) {
	// Reproduces a confirmed vulnerability: a text/plain POST - which a
	// browser sends cross-site without any CORS preflight - used to be
	// decoded and acted on.
	s := &Server{Version: "dev", Hosts: openTestHosts(t)}
	body := `{"name":"evil","base_url":"http://127.0.0.1:1"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/register-host", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
	if hosts, _ := s.Hosts.Hosts(); len(hosts) != 0 {
		t.Fatalf("a rejected request must not register anything, got %+v", hosts)
	}
}

func TestHandleHealth_ReportsCommandRegistration(t *testing.T) {
	s := &Server{Version: "dev", Hosts: openTestHosts(t), AllowCommandRegistration: true}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/health", nil))
	var got HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.CommandRegistration {
		t.Fatalf("health = %+v, want command_registration=true", got)
	}
}

func TestAdmin_NeverReturnsAPIKeys(t *testing.T) {
	hosts := openTestHosts(t)
	backends := backend.NewRegistry([]config.Backend{{Name: "remote", BaseURL: "https://api.example/v1", APIKey: "backend-secret"}})
	s := &Server{Version: "dev", Hosts: hosts, Backends: backends}

	body, _ := json.Marshal(RegisterHostRequest{Name: "rx9070", BaseURL: "http://192.168.18.29:11434", APIKey: "host-secret"})
	req := httptest.NewRequest(http.MethodPost, "/admin/register-host", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d: %s", rec.Code, rec.Body.String())
	}
	responses := rec.Body.String()

	for _, path := range []string{"/admin/hosts", "/admin/backends"} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		responses += rec.Body.String()
	}
	for _, secret := range []string{"host-secret", "backend-secret"} {
		if strings.Contains(responses, secret) {
			t.Fatalf("admin responses leak %q: %s", secret, responses)
		}
	}
	if !strings.Contains(responses, `"has_api_key":true`) {
		t.Fatalf("responses should still say a key is set: %s", responses)
	}

	// Editing the host without re-sending its key keeps the stored key.
	body, _ = json.Marshal(RegisterHostRequest{Name: "rx9070", BaseURL: "http://192.168.18.30:11434"})
	req = httptest.NewRequest(http.MethodPost, "/admin/register-host", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	stored, _ := hosts.Hosts()
	if len(stored) != 1 || stored[0].APIKey != "host-secret" || stored[0].BaseURL != "http://192.168.18.30:11434" {
		t.Fatalf("stored host after keyless edit = %+v", stored)
	}
}
