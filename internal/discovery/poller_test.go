package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
)

func TestPollOnce_DiscoversModelsAsBackends(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b","capabilities":["completion","tools"]}]}`))
	}))
	defer srv.Close()

	hostsPath := filepath.Join(t.TempDir(), "hosts.db")
	hosts, err := hostregistry.Open(hostsPath)
	if err != nil {
		t.Fatalf("hostregistry.Open: %v", err)
	}
	defer func() { _ = hosts.Close() }()

	host := hostregistry.Host{Name: "rx9070", BaseURL: srv.URL}
	if err := hosts.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	backends := backend.NewRegistry(nil)
	p := NewPoller(hosts, backends)
	p.PollOnce(context.Background(), host)

	got, err := backends.Get("rx9070/gemma4:12b")
	if err != nil {
		t.Fatalf("Get(rx9070/gemma4:12b): %v", err)
	}
	if got.Model != "gemma4:12b" {
		t.Fatalf("got.Model = %q, want gemma4:12b", got.Model)
	}
	if got.BaseURL != srv.URL+"/v1" {
		t.Fatalf("got.BaseURL = %q, want %q", got.BaseURL, srv.URL+"/v1")
	}

	st, ok := hosts.StatusOf(host.Name)
	if !ok || !st.Up {
		t.Fatalf("host status = %+v, ok=%v, want Up=true", st, ok)
	}
}

func TestPollOnce_ModelRemovedLocallyDropsItsBackendOnNextSuccessfulPoll(t *testing.T) {
	// Reproduces a real reported bug: deleting a model on the Ollama host
	// (while the host stays reachable) must make it stop being a usable
	// backend as of the very next poll - it must not linger just because
	// the host itself is still up.
	var modelsResponse atomic.Value
	modelsResponse.Store(`{"models":[{"name":"gemma4:12b"},{"name":"qwen3.5:latest"}]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsResponse.Load().(string)))
	}))
	defer srv.Close()

	hostsPath := filepath.Join(t.TempDir(), "hosts.db")
	hosts, err := hostregistry.Open(hostsPath)
	if err != nil {
		t.Fatalf("hostregistry.Open: %v", err)
	}
	defer func() { _ = hosts.Close() }()

	host := hostregistry.Host{Name: "rx9070", BaseURL: srv.URL}
	if err := hosts.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	backends := backend.NewRegistry(nil)
	p := NewPoller(hosts, backends)
	p.PollOnce(context.Background(), host)

	if _, err := backends.Get("rx9070/gemma4:12b"); err != nil {
		t.Fatalf("Get(rx9070/gemma4:12b) after first poll: %v", err)
	}
	if _, err := backends.Get("rx9070/qwen3.5:latest"); err != nil {
		t.Fatalf("Get(rx9070/qwen3.5:latest) after first poll: %v", err)
	}

	// The user deleted qwen3.5:latest locally; the host is still up and
	// now only reports gemma4:12b.
	modelsResponse.Store(`{"models":[{"name":"gemma4:12b"}]}`)
	p.PollOnce(context.Background(), host)

	if _, err := backends.Get("rx9070/gemma4:12b"); err != nil {
		t.Fatalf("Get(rx9070/gemma4:12b) after second poll: %v", err)
	}
	if _, err := backends.Get("rx9070/qwen3.5:latest"); err == nil {
		t.Fatalf("rx9070/qwen3.5:latest should have been removed after it disappeared from a successful poll")
	}

	st, ok := hosts.StatusOf(host.Name)
	if !ok || !st.Up {
		t.Fatalf("host status = %+v, ok=%v, want Up=true (host is still reachable)", st, ok)
	}
	if len(st.Models) != 1 || st.Models[0].Name != "gemma4:12b" {
		t.Fatalf("st.Models = %+v, want only [gemma4:12b]", st.Models)
	}
}

func TestPollOnce_UnreachableHostRemovesSynthesizedBackends(t *testing.T) {
	hostsPath := filepath.Join(t.TempDir(), "hosts.db")
	hosts, err := hostregistry.Open(hostsPath)
	if err != nil {
		t.Fatalf("hostregistry.Open: %v", err)
	}
	defer func() { _ = hosts.Close() }()

	// A closed server: connections fail immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
	}))
	host := hostregistry.Host{Name: "rx9070", BaseURL: srv.URL}
	if err := hosts.RegisterHost(host); err != nil {
		t.Fatalf("RegisterHost: %v", err)
	}

	backends := backend.NewRegistry(nil)
	p := NewPoller(hosts, backends)
	p.PollOnce(context.Background(), host) // first poll succeeds, registers the backend
	if _, err := backends.Get("rx9070/gemma4:12b"); err != nil {
		t.Fatalf("Get after first poll: %v", err)
	}

	srv.Close() // now the host is unreachable
	p.PollOnce(context.Background(), host)

	if _, err := backends.Get("rx9070/gemma4:12b"); err == nil {
		t.Fatalf("Get after failed poll should error, backend should have been removed")
	}

	st, ok := hosts.StatusOf(host.Name)
	if !ok || st.Up {
		t.Fatalf("host status = %+v, ok=%v, want Up=false after unreachable poll", st, ok)
	}
}
