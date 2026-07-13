// Package backend implements the OpenAI-compatible backend registry and the
// thin HTTP client used to talk to it (local llama.cpp/Ollama servers, or
// any remote provider exposing the same /chat/completions shape).
package backend

import (
	"fmt"
	"sync"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

// Registry holds the configured backends, looked up by name. The first
// statically-configured backend is the default used when no name is given.
// Safe for concurrent use: entries can be added or removed at runtime by a
// host-discovery poller while tool calls are resolving names concurrently.
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

// Get returns the named backend, or the first statically-configured backend
// when name is empty.
func (r *Registry) Get(name string) (config.Backend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		if len(r.order) == 0 {
			return config.Backend{}, fmt.Errorf("no backends configured")
		}
		name = r.order[0]
	}
	b, ok := r.backends[name]
	if !ok {
		return config.Backend{}, fmt.Errorf("unknown backend %q", name)
	}
	return b, nil
}

// List returns all currently known backends (static and dynamically
// registered), static ones first in config order.
func (r *Registry) List() []config.Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]config.Backend, 0, len(r.backends))
	seen := make(map[string]bool, len(r.order))
	for _, name := range r.order {
		out = append(out, r.backends[name])
		seen[name] = true
	}
	for name, b := range r.backends {
		if !seen[name] {
			out = append(out, b)
		}
	}
	return out
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
