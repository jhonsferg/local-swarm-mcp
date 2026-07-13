// Package mcpserverregistry persists registered downstream MCP servers
// (e.g. codebase-memory-mcp) in bbolt, the same way internal/hostregistry
// persists inference hosts - so a tool-using agent's downstream server
// list can be managed at runtime (register_downstream_mcp_server /
// unregister_downstream_mcp_server) instead of only via a config file.
package mcpserverregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

var bucketName = []byte("mcp_servers")

// Server is a registered downstream MCP server, spawned as a local stdio
// subprocess (matching config.MCPServer's shape).
type Server struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// Registry persists registered downstream MCP servers.
type Registry struct {
	db *bolt.DB
}

// Open opens (creating if needed) the bbolt database at path.
func Open(path string) (*Registry, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create mcp server registry directory: %w", err)
		}
	}

	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open mcp server registry: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init mcp server registry bucket: %w", err)
	}
	return &Registry{db: db}, nil
}

// Close releases the underlying database file.
func (r *Registry) Close() error {
	return r.db.Close()
}

// Register persists a downstream MCP server definition.
func (r *Registry) Register(s Server) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Put([]byte(s.Name), data)
	}); err != nil {
		return fmt.Errorf("persist mcp server %q: %w", s.Name, err)
	}
	return nil
}

// Unregister removes a previously registered downstream MCP server.
func (r *Registry) Unregister(name string) error {
	if err := r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Delete([]byte(name))
	}); err != nil {
		return fmt.Errorf("remove mcp server %q: %w", name, err)
	}
	return nil
}

// List returns every registered downstream MCP server.
func (r *Registry) List() ([]Server, error) {
	var out []Server
	err := r.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).ForEach(func(k, v []byte) error {
			var s Server
			if err := json.Unmarshal(v, &s); err != nil {
				return fmt.Errorf("decode mcp server %q: %w", k, err)
			}
			out = append(out, s)
			return nil
		})
	})
	return out, err
}

// DefaultPath returns the registry's database location when none is
// overridden.
func DefaultPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "local-swarm-mcp-mcp-servers.db"
	}
	return filepath.Join(dir, "local-swarm-mcp", "mcp-servers.db")
}
