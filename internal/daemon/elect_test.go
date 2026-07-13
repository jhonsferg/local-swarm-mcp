package daemon

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/daemonlock"
)

func TestElect_NoExistingLock_BecomesDaemon(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")

	role, lock, err := Elect(path, func() error { t.Fatal("healthCheck should not be called"); return nil })
	if err != nil {
		t.Fatalf("Elect: %v", err)
	}
	if role != RoleDaemon {
		t.Fatalf("role = %v, want RoleDaemon", role)
	}
	if lock == nil {
		t.Fatalf("lock = nil, want a lock to release on shutdown")
	}
	_ = lock.Release()
}

func TestElect_HealthyExistingDaemon_BecomesClient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")

	// Simulate another live process already holding the lock: acquire it
	// under our own PID (guaranteed live for the test's duration), then
	// elect again without releasing.
	holder, err := daemonlock.TryAcquire(path)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	defer func() { _ = holder.Release() }()

	role, lock, err := Elect(path, func() error { return nil })
	if err != nil {
		t.Fatalf("Elect: %v", err)
	}
	if role != RoleClient {
		t.Fatalf("role = %v, want RoleClient", role)
	}
	if lock != nil {
		t.Fatalf("lock = %+v, want nil for RoleClient", lock)
	}
}

func TestElect_LiveLockButUnhealthy_IsAConflictError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")

	holder, err := daemonlock.TryAcquire(path)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	defer func() { _ = holder.Release() }()

	wantErr := errors.New("connection refused")
	_, _, err = Elect(path, func() error { return wantErr })
	if err == nil {
		t.Fatalf("Elect should error when the live lock holder doesn't answer health checks")
	}
}
