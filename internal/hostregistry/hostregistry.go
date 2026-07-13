// Package hostregistry persists registered inference hosts (e.g. a remote
// Ollama instance on a desktop GPU, a DGX Spark, an AMD AI Halo box) and the
// models discovered on them, so a service-discovery poller can keep the
// backend registry in sync without any YAML/JSON config editing or process
// restart. Backed by bbolt (already a dependency, pure Go, no cgo).
package hostregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	hostsBucket  = []byte("hosts")
	modelsBucket = []byte("models")
)

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

// HostStatus is a Host plus the poller's live view of it: the models last
// seen there, and whether the most recent poll succeeded.
type HostStatus struct {
	Host
	Models   []Model   `json:"models"`
	Up       bool      `json:"up"`
	LastSeen time.Time `json:"last_seen"`
	LastErr  string    `json:"last_error,omitempty"`
}

// Registry persists hosts and their discovered models, and keeps a live
// in-memory status view (Up/LastSeen/LastErr) that the poller updates -
// that live view is intentionally not persisted, since "was this host up
// five minutes before the process last exited" isn't meaningful.
type Registry struct {
	db *bolt.DB

	mu     sync.RWMutex
	status map[string]HostStatus // name -> live status, seeded from bbolt at Open
}

// Open opens (creating if needed) the bbolt database at path and loads any
// previously registered hosts into the live status map (as not-yet-polled,
// Up=false, until the poller's first successful check).
func Open(path string) (*Registry, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create host registry directory: %w", err)
		}
	}

	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open host registry: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(hostsBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(modelsBucket)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init host registry buckets: %w", err)
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
			models, err := r.modelsForTx(tx, h.Name)
			if err != nil {
				return err
			}
			r.status[h.Name] = HostStatus{Host: h, Models: models}
			return nil
		})
	})
}

func (r *Registry) modelsForTx(tx *bolt.Tx, hostName string) ([]Model, error) {
	v := tx.Bucket(modelsBucket).Get([]byte(hostName))
	if v == nil {
		return nil, nil
	}
	var models []Model
	if err := json.Unmarshal(v, &models); err != nil {
		return nil, fmt.Errorf("decode models for host %q: %w", hostName, err)
	}
	return models, nil
}

// Close releases the underlying database file.
func (r *Registry) Close() error {
	return r.db.Close()
}

// RegisterHost persists a host and makes it immediately visible in the live
// status view (Up=false until the poller checks it for the first time).
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

// UnregisterHost removes a host and its discovered models.
func (r *Registry) UnregisterHost(name string) error {
	if err := r.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(hostsBucket).Delete([]byte(name)); err != nil {
			return err
		}
		return tx.Bucket(modelsBucket).Delete([]byte(name))
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

// RecordPoll updates the live status and, when models is non-nil (a
// successful poll), persists the discovered model list.
func (r *Registry) RecordPoll(hostName string, models []Model, pollErr error) error {
	if models != nil {
		data, err := json.Marshal(models)
		if err != nil {
			return err
		}
		if err := r.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(modelsBucket).Put([]byte(hostName), data)
		}); err != nil {
			return fmt.Errorf("persist models for host %q: %w", hostName, err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.status[hostName]
	st.LastSeen = time.Now()
	if pollErr != nil {
		st.Up = false
		st.LastErr = pollErr.Error()
	} else {
		st.Up = true
		st.LastErr = ""
		if models != nil {
			st.Models = models
		}
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
