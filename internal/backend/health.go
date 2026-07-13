package backend

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

// HealthStatus is the result of probing a backend's reachability.
type HealthStatus struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail,omitempty"`
}

// Check probes a backend's /models endpoint (part of the OpenAI-compatible
// API surface) to confirm it's reachable before a caller attempts to
// delegate real work to it.
func Check(ctx context.Context, b config.Backend) HealthStatus {
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimSuffix(b.BaseURL, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HealthStatus{Name: b.Name, Healthy: false, Detail: err.Error()}
	}
	if b.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return HealthStatus{Name: b.Name, Healthy: false, Detail: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return HealthStatus{Name: b.Name, Healthy: true}
	}
	return HealthStatus{Name: b.Name, Healthy: false, Detail: fmt.Sprintf("status %d", resp.StatusCode)}
}
