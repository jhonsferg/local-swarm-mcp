// Package backend implements the OpenAI-compatible backend registry and the
// thin HTTP client used to talk to it (local llama.cpp/Ollama servers, or
// any remote provider exposing the same /chat/completions shape).
package backend

import (
	"fmt"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

// Registry holds the configured backends, looked up by name. The first
// backend in config order is the default used when no name is given.
type Registry struct {
	backends map[string]config.Backend
	order    []string
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

// Get returns the named backend, or the first configured backend when name
// is empty.
func (r *Registry) Get(name string) (config.Backend, error) {
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

// List returns all configured backends in config order.
func (r *Registry) List() []config.Backend {
	out := make([]config.Backend, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.backends[name])
	}
	return out
}
