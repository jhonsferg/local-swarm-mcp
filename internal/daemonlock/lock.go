// Package daemonlock implements the leader-election lockfile used to decide
// which of possibly several concurrently-started local-swarm-mcp processes
// becomes the one persistent daemon. A stale lock (left behind by a daemon
// that crashed without cleaning up) is detected via OS process liveness and
// reclaimed automatically; a live lock holder that isn't actually
// responding as a real local-swarm-mcp daemon is left alone and reported as
// a genuine conflict rather than silently overridden.
package daemonlock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Info is the content written into the lock file.
type Info struct {
	PID int `json:"pid"`
}

// Lock represents an acquired lockfile. Call Release when the process that
// acquired it is shutting down.
type Lock struct {
	path string
}

// ErrHeld is returned by TryAcquire when a live process already holds the
// lock. Its PID is included so the caller can confirm (e.g. via an HTTP
// health check) that it is actually a functioning local-swarm-mcp daemon
// before deciding how to proceed.
type ErrHeld struct {
	PID int
}

func (e *ErrHeld) Error() string {
	return fmt.Sprintf("lock held by live process %d", e.PID)
}

// TryAcquire attempts to acquire the lockfile at path.
//
// If the file doesn't exist, or exists but names a PID that is no longer
// running (a stale lock from an unclean shutdown), it is created/overwritten
// with the current process's PID and acquisition succeeds. If it names a
// PID that is still alive, TryAcquire returns *ErrHeld without touching the
// file - the caller should confirm via an independent liveness signal
// (health check) before treating that PID as a legitimate daemon.
func TryAcquire(path string) (*Lock, error) {
	if existing, err := readInfo(path); err == nil {
		if processAlive(existing.PID) {
			return nil, &ErrHeld{PID: existing.PID}
		}
		// Stale: the lock file's PID is dead. Fall through to reclaim it.
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create lock directory: %w", err)
		}
	}

	data, err := json.Marshal(Info{PID: os.Getpid()})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write lock file %s: %w", path, err)
	}
	return &Lock{path: path}, nil
}

// Release removes the lock file. Safe to call even if it no longer exists.
func (l *Lock) Release() error {
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readInfo(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("corrupt lock file %s: %w", path, err)
	}
	return info, nil
}

// PIDString is a small helper for log messages.
func (i Info) PIDString() string {
	return strconv.Itoa(i.PID)
}
