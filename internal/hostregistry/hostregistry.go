// Package hostregistry persists registered inference hosts (e.g. a remote
// Ollama instance on a desktop GPU, a DGX Spark, an AMD AI Halo box) so a
// service-discovery poller can keep the backend registry in sync without
// any YAML/JSON config editing or process restart. Backed by bbolt
// (already a dependency, pure Go, no cgo).
//
// Only the host itself (name, base URL, API key) is persisted. The models
// discovered on it are never written to disk - they live purely in
// memory, refreshed by each real poll of the host's own API, so a model
// you deleted locally stops showing up as soon as the next poll runs
// rather than lingering from a stale on-disk snapshot until something
// happens to overwrite it.
package hostregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var hostsBucket = []byte("hosts")

// Host is a registered inference endpoint - a machine reachable over the
// network (or localhost) that speaks Ollama's API.
type Host struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key,omitempty"`
}

// Model is a single model discovered on a host by the polling loop.
type Model struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// HostStatus is a Host plus the poller's live, in-memory-only view of it:
// the models seen on its most recent successful poll, and whether that
// poll succeeded.
type HostStatus struct {
	Host
	Models   []Model   `json:"models"`
	Up       bool      `json:"up"`
	LastSeen time.Time `json:"last_seen"`
	LastErr  string    `json:"last_error,omitempty"`
}

// Registry persists hosts and keeps a live in-memory status view
// (Models/Up/LastSeen/LastErr) that the poller updates - none of that
// live view is persisted, since a stale on-disk model list would be
// exactly the kind of caching a "what's actually there right now" view
// isn't supposed to have.
type Registry struct {
	db *bolt.DB

	mu     sync.RWMutex
	status map[string]HostStatus // name -> live status, seeded from bbolt at Open (Models empty until the first poll)
}

// Open opens (creating if needed) the bbolt database at path and loads any
// previously registered hosts into the live status map (as not-yet-polled,
// Up=false, no models, until the poller's first check). Blocks indefinitely
// if another process already holds the file lock - see OpenWithOptions to
// bound that wait.
func Open(path string) (*Registry, error) {
	return OpenWithOptions(path, nil)
}

// OpenWithOptions is Open with caller-supplied bbolt options - notably
// Options.Timeout, so a caller that may race another local-swarm-mcp
// process for this same store (e.g. a stdio session opportunistically
// sharing the daemon's store) can fail fast instead of hanging forever
// waiting for the other process to release the file lock.
func OpenWithOptions(path string, opts *bolt.Options) (*Registry, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create host registry directory: %w", err)
		}
	}

	db, err := bolt.Open(path, 0o600, opts)
	if err != nil {
		return nil, fmt.Errorf("open host registry: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(hostsBucket)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init host registry bucket: %w", err)
	}

	r := &Registry{db: db, status: make(map[string]HostStatus)}
	if err := r.loadIntoStatus(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Registry) loadIntoStatus() error {
	return r.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(hostsBucket).ForEach(func(k, v []byte) error {
			var h Host
			if err := json.Unmarshal(v, &h); err != nil {
				return fmt.Errorf("decode host %q: %w", k, err)
			}
			r.status[h.Name] = HostStatus{Host: h}
			return nil
		})
	})
}

// Close releases the underlying database file.
func (r *Registry) Close() error {
	return r.db.Close()
}

// RegisterHost persists a host and makes it immediately visible in the live
// status view (Up=false, no models, until the poller checks it for the
// first time). Calling this again for an existing name updates its
// base URL/API key (upsert), which is also how editing a registration
// works - there's no separate "update" operation.
func (r *Registry) RegisterHost(h Host) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if err := r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(hostsBucket).Put([]byte(h.Name), data)
	}); err != nil {
		return fmt.Errorf("persist host %q: %w", h.Name, err)
	}

	r.mu.Lock()
	existing := r.status[h.Name]
	existing.Host = h
	r.status[h.Name] = existing
	r.mu.Unlock()
	return nil
}

// UnregisterHost removes a host.
func (r *Registry) UnregisterHost(name string) error {
	if err := r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(hostsBucket).Delete([]byte(name))
	}); err != nil {
		return fmt.Errorf("remove host %q: %w", name, err)
	}

	r.mu.Lock()
	delete(r.status, name)
	r.mu.Unlock()
	return nil
}

// Hosts returns every registered host (not the live status - see Status).
func (r *Registry) Hosts() ([]Host, error) {
	var hosts []Host
	err := r.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(hostsBucket).ForEach(func(k, v []byte) error {
			var h Host
			if err := json.Unmarshal(v, &h); err != nil {
				return fmt.Errorf("decode host %q: %w", k, err)
			}
			hosts = append(hosts, h)
			return nil
		})
	})
	return hosts, err
}

// ErrUnknownHost is returned by RecordPoll for a host that isn't (or is no
// longer) registered.
var ErrUnknownHost = errors.New("host is not registered")

// RecordPoll updates the live in-memory status with the outcome of a real
// poll. Nothing here touches disk - models is never persisted, so a model
// removed on the host itself stops showing up as of the very next poll.
// A poll that finishes after its host was unregistered is discarded with
// ErrUnknownHost, rather than resurrecting a status entry for it.
func (r *Registry) RecordPoll(hostName string, models []Model, pollErr error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.status[hostName]
	if !ok {
		return fmt.Errorf("record poll for %q: %w", hostName, ErrUnknownHost)
	}
	st.LastSeen = time.Now()
	if pollErr != nil {
		st.Up = false
		st.LastErr = pollErr.Error()
	} else {
		st.Up = true
		st.LastErr = ""
		st.Models = models
	}
	r.status[hostName] = st
	return nil
}

// Status returns the live view of every registered host.
func (r *Registry) Status() []HostStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HostStatus, 0, len(r.status))
	for _, st := range r.status {
		out = append(out, st)
	}
	return out
}

// StatusOf returns the live view of a single host, if registered.
func (r *Registry) StatusOf(name string) (HostStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.status[name]
	return st, ok
}

// Upsert registers h like RegisterHost, except that an empty h.APIKey keeps
// the key already stored for an existing host of that name instead of
// clearing it - so a client that never sees keys (see View) can still edit
// a host's base URL without re-entering its key. Returns the host as
// stored.
func (r *Registry) Upsert(h Host) (Host, error) {
	if h.APIKey == "" {
		if existing, ok := r.StatusOf(h.Name); ok {
			h.APIKey = existing.APIKey
		}
	}
	return h, r.RegisterHost(h)
}

// HostView is a HostStatus as shown to clients (MCP tools, the admin API,
// the dashboard): everything except the API key itself, which is only
// ever reported as present or not.
type HostView struct {
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	HasAPIKey bool      `json:"has_api_key"`
	Models    []Model   `json:"models"`
	Up        bool      `json:"up"`
	LastSeen  time.Time `json:"last_seen"`
	LastErr   string    `json:"last_error,omitempty"`
}

// View returns st without its API key.
func (st HostStatus) View() HostView {
	return HostView{
		Name:      st.Name,
		BaseURL:   st.BaseURL,
		HasAPIKey: st.APIKey != "",
		Models:    st.Models,
		Up:        st.Up,
		LastSeen:  st.LastSeen,
		LastErr:   st.LastErr,
	}
}

// Views returns the redacted live view of every registered host.
func (r *Registry) Views() []HostView {
	statuses := r.Status()
	out := make([]HostView, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, st.View())
	}
	return out
}
