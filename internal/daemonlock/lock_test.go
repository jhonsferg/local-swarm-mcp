package daemonlock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTryAcquire_FreshLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "daemon.lock")

	lock, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	info, err := readInfo(path)
	if err != nil {
		t.Fatalf("readInfo: %v", err)
	}
	if info.PID != os.Getpid() {
		t.Fatalf("PID = %d, want %d", info.PID, os.Getpid())
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file still exists after Release")
	}
}

func TestTryAcquire_HeldByLiveProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")

	// Our own PID is guaranteed live for the duration of this test.
	writeLockFile(t, path, os.Getpid())

	_, err := TryAcquire(path)
	var held *ErrHeld
	if !errors.As(err, &held) {
		t.Fatalf("TryAcquire = %v, want *ErrHeld", err)
	}
	if held.PID != os.Getpid() {
		t.Fatalf("ErrHeld.PID = %d, want %d", held.PID, os.Getpid())
	}
}

func TestTryAcquire_ReclaimsStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")

	deadPID := spawnAndWaitExit(t)
	writeLockFile(t, path, deadPID)

	lock, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire should reclaim a stale lock, got: %v", err)
	}
	defer func() { _ = lock.Release() }()

	info, err := readInfo(path)
	if err != nil {
		t.Fatalf("readInfo: %v", err)
	}
	if info.PID != os.Getpid() {
		t.Fatalf("lock file PID = %d after reclaim, want %d", info.PID, os.Getpid())
	}
}

func writeLockFile(t *testing.T, path string, pid int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data := []byte(`{"pid":` + itoa(pid) + `}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func itoa(n int) string {
	return (Info{PID: n}).PIDString()
}

// spawnAndWaitExit starts and waits for a trivial short-lived child process,
// returning its now-dead PID - a reliable stand-in for "a PID that used to
// exist but doesn't anymore" without relying on PID reuse timing.
func spawnAndWaitExit(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn helper process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait helper process: %v", err)
	}
	return pid
}
