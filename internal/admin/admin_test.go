package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
)

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
	var got []hostregistry.HostStatus
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
