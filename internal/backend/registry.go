// Package backend implements the OpenAI-compatible backend registry and the
// thin HTTP client used to talk to it (local llama.cpp/Ollama servers, or
// any remote provider exposing the same /chat/completions shape).
package backend

import (
	"fmt"
	"sort"
	"sync"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

// Registry holds the configured backends, looked up by name. The first
// statically-configured backend is the default used when no name is given;
// with no static backends at all, the default is the first runtime-added
// (discovered) backend in name order, so a host-discovery-only setup still
// works without naming a backend on every call. Safe for concurrent use:
// entries can be added or removed at runtime by a host-discovery poller
// while tool calls are resolving names concurrently.
type Registry struct {
	mu       sync.RWMutex
	backends map[string]config.Backend
	order    []string // static (config-file) order; the default-lookup anchor
}

// NewRegistry builds a Registry from the configured backend list.
func NewRegistry(backends []config.Backend) *Registry {
	r := &Registry{backends: make(map[string]config.Backend, len(backends))}
	for _, b := range backends {
		r.backends[b.Name] = b
		r.order = append(r.order, b.Name)
	}
	return r
}

// Get returns the named backend, or the default backend when name is
// empty: the first statically-configured backend, or - if there are none -
// the first runtime-added backend in name order (see List).
func (r *Registry) Get(name string) (config.Backend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		switch {
		case len(r.order) > 0:
			name = r.order[0]
		case len(r.backends) > 0:
			name = r.dynamicNames()[0]
		default:
			return config.Backend{}, fmt.Errorf("no backends configured")
		}
	}
	b, ok := r.backends[name]
	if !ok {
		return config.Backend{}, fmt.Errorf("unknown backend %q", name)
	}
	return b, nil
}

// List returns all currently known backends (static and dynamically
// registered): static ones first in config order, then dynamic ones in
// name order - so the first entry is always the default Get("") resolves.
func (r *Registry) List() []config.Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]config.Backend, 0, len(r.backends))
	for _, name := range r.order {
		out = append(out, r.backends[name])
	}
	for _, name := range r.dynamicNames() {
		out = append(out, r.backends[name])
	}
	return out
}

// dynamicNames returns the names of every backend not in the static
// config order, sorted. Callers must hold r.mu.
func (r *Registry) dynamicNames() []string {
	static := make(map[string]bool, len(r.order))
	for _, name := range r.order {
		static[name] = true
	}
	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		if !static[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Put adds or replaces a backend entry at runtime (e.g. one synthesized
// from a discovered host+model pair). It does not affect the static
// default-lookup order.
func (r *Registry) Put(b config.Backend) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[b.Name] = b
}

// Remove deletes a runtime-registered backend entry, if present.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.backends, name)
}

// Info is a backend as shown to clients (MCP tools, the admin API, the
// dashboard): everything except the API key itself.
type Info struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"has_api_key"`
}

// Infos returns List() without API keys.
func (r *Registry) Infos() []Info {
	backends := r.List()
	out := make([]Info, 0, len(backends))
	for _, b := range backends {
		out = append(out, Info{Name: b.Name, BaseURL: b.BaseURL, Model: b.Model, HasAPIKey: b.APIKey != ""})
	}
	return out
}
