package mcpserverregistry

import (
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func openTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := Open(filepath.Join(t.TempDir(), "mcp-servers.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRegisterAndList(t *testing.T) {
	r := openTestRegistry(t)
	srv := Server{Name: "codebase-memory-mcp", Command: "C:/tools/codebase-memory-mcp.exe"}
	if err := r.Register(srv); err != nil {
		t.Fatalf("Register: %v", err)
	}

	servers, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != srv.Name || servers[0].Command != srv.Command {
		t.Fatalf("List() = %+v, want [%+v]", servers, srv)
	}
}

func TestUnregister(t *testing.T) {
	r := openTestRegistry(t)
	if err := r.Register(Server{Name: "codebase-memory-mcp", Command: "cmd"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Unregister("codebase-memory-mcp"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	servers, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("List() after unregister = %+v, want empty", servers)
	}
}

func TestReopenPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-servers.db")
	r1, err := Open(path)
	if err != nil {
		t.Fatalf("Open (1st): %v", err)
	}
	if err := r1.Register(Server{Name: "sonar-bridge-mcp", Command: "sonar-bridge-mcp.exe", Args: []string{"-x"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r2, err := Open(path)
	if err != nil {
		t.Fatalf("Open (2nd): %v", err)
	}
	defer func() { _ = r2.Close() }()

	servers, err := r2.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "sonar-bridge-mcp" || len(servers[0].Args) != 1 {
		t.Fatalf("List() after reopen = %+v", servers)
	}
}

// TestOpenWithOptions_TimesOutWhenAlreadyLocked mirrors hostregistry's
// equivalent test - a stdio session opportunistically sharing the daemon's
// downstream-MCP-server store must fail fast, not hang, if the daemon
// already holds the file lock.
func TestOpenWithOptions_TimesOutWhenAlreadyLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-servers.db")

	holder, err := Open(path)
	if err != nil {
		t.Fatalf("Open (holder): %v", err)
	}
	defer func() { _ = holder.Close() }()

	start := time.Now()
	_, err = OpenWithOptions(path, &bolt.Options{Timeout: 100 * time.Millisecond})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("OpenWithOptions succeeded despite the file already being held open")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("OpenWithOptions took %s to fail - timeout was not respected", elapsed)
	}
}
