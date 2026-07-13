// Package daemon implements the leader-election decision used by both a
// plain daemon startup and "--register-host": acquire a lockfile, and if
// another process already holds it, confirm via an HTTP health check that
// it's actually a live, responsive local-swarm-mcp daemon (not just a
// stale lock or an unrelated process) before deferring to it.
package daemon

import (
	"errors"
	"fmt"

	"github.com/jhonsferg/local-swarm-mcp/internal/daemonlock"
)

// Role is the outcome of an election.
type Role int

const (
	// RoleDaemon means this process won the lock and should become the
	// persistent daemon.
	RoleDaemon Role = iota
	// RoleClient means a healthy daemon is already running; this process
	// should act as a thin client to it instead.
	RoleClient
)

// Elect attempts to acquire lockPath. If it's already held by a live
// process, healthCheck is called to confirm that process is actually a
// responsive local-swarm-mcp daemon before conceding RoleClient - a live
// PID that doesn't answer correctly is a genuine, surfaced conflict, not
// something to silently paper over.
//
// On RoleDaemon, the returned *daemonlock.Lock must be released by the
// caller when the daemon shuts down.
func Elect(lockPath string, healthCheck func() error) (Role, *daemonlock.Lock, error) {
	lock, err := daemonlock.TryAcquire(lockPath)
	if err == nil {
		return RoleDaemon, lock, nil
	}

	var held *daemonlock.ErrHeld
	if !errors.As(err, &held) {
		return 0, nil, fmt.Errorf("acquire daemon lock %s: %w", lockPath, err)
	}

	if hcErr := healthCheck(); hcErr != nil {
		return 0, nil, fmt.Errorf(
			"lock file %s is held by live process %d, but it isn't answering as a local-swarm-mcp daemon (%v) - "+
				"if that process really is gone, remove the lock file and try again",
			lockPath, held.PID, hcErr,
		)
	}
	return RoleClient, nil, nil
}
