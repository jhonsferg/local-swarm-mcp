// Package discovery periodically polls every registered host's Ollama
// /api/tags endpoint, updates the host registry with what it finds, and
// mirrors the result into the backend registry as synthesized
// "<host>/<model>" entries - so newly discovered models become usable by
// delegate_task/spawn_agent_task's plain "backend" string parameter without
// any config file edit or process restart.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
	"github.com/jhonsferg/local-swarm-mcp/internal/hostregistry"
)

// DefaultInterval is how often each registered host is polled.
const DefaultInterval = 30 * time.Second

// Poller owns the background polling loop for every registered host.
type Poller struct {
	Hosts    *hostregistry.Registry
	Backends *backend.Registry
	Interval time.Duration
	client   *http.Client
}

// NewPoller builds a Poller with a sane HTTP timeout (polling a host that's
// merely powered off must fail fast, not hang the whole loop).
func NewPoller(hosts *hostregistry.Registry, backends *backend.Registry) *Poller {
	interval := DefaultInterval
	return &Poller{
		Hosts:    hosts,
		Backends: backends,
		Interval: interval,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Run polls every registered host once immediately, then on Interval, until
// ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	p.pollAll(ctx)
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	hosts, err := p.Hosts.Hosts()
	if err != nil {
		return
	}
	for _, h := range hosts {
		p.pollOne(ctx, h)
	}
}

// PollOnce polls a single host immediately, outside the regular ticker -
// used to make a newly registered host's models available right away
// instead of waiting up to Interval for the first check.
func (p *Poller) PollOnce(ctx context.Context, h hostregistry.Host) {
	p.pollOne(ctx, h)
}

func (p *Poller) pollOne(ctx context.Context, h hostregistry.Host) {
	models, err := fetchTags(ctx, p.client, h)
	_ = p.Hosts.RecordPoll(h.Name, modelsOrNil(models, err), err)

	if err != nil {
		// Host unreachable (e.g. powered off): drop its synthesized
		// backends so nothing tries to dispatch work there, but keep the
		// host registered - it may come back on the next poll.
		for _, m := range lastKnownModels(p.Hosts, h.Name) {
			p.Backends.Remove(backendName(h.Name, m.Name))
		}
		return
	}

	for _, m := range models {
		p.Backends.Put(config.Backend{
			Name:    backendName(h.Name, m.Name),
			BaseURL: strings.TrimRight(h.BaseURL, "/") + "/v1",
			Model:   m.Name,
			APIKey:  h.APIKey,
		})
	}
}

func modelsOrNil(models []hostregistry.Model, err error) []hostregistry.Model {
	if err != nil {
		return nil
	}
	return models
}

func lastKnownModels(hosts *hostregistry.Registry, hostName string) []hostregistry.Model {
	st, ok := hosts.StatusOf(hostName)
	if !ok {
		return nil
	}
	return st.Models
}

// backendName synthesizes the composite backend name a tool call uses to
// target a specific model on a specific host.
func backendName(hostName, modelName string) string {
	return hostName + "/" + modelName
}

type tagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Details struct {
			Capabilities []string `json:"capabilities"`
		} `json:"details"`
		Capabilities []string `json:"capabilities"`
	} `json:"models"`
}

// fetchTags calls Ollama's native /api/tags on the host and returns the
// models it reports.
func fetchTags(ctx context.Context, client *http.Client, h hostregistry.Host) ([]hostregistry.Model, error) {
	url := strings.TrimRight(h.BaseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	var tr tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode /api/tags response: %w", err)
	}

	models := make([]hostregistry.Model, 0, len(tr.Models))
	for _, m := range tr.Models {
		caps := m.Capabilities
		if len(caps) == 0 {
			caps = m.Details.Capabilities
		}
		models = append(models, hostregistry.Model{Name: m.Name, Capabilities: caps})
	}
	return models, nil
}
