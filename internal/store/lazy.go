package store

import (
	"errors"
	"fmt"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

// ErrBusy is returned by Lazy.Get while another process holds the store's
// file lock.
var ErrBusy = errors.New("scratch store is held by another local-swarm-mcp process")

// Lazy opens a Store on first use instead of at process startup, with a
// bounded lock wait, retrying on every later call until it succeeds.
//
// bbolt allows only one process to hold a database file open at a time,
// and every local-swarm-mcp process (a -transport http daemon, each stdio
// session an MCP client spawns, a short-lived -register-host client)
// shares the same default scratch-store path. Opening eagerly with no
// timeout made any second process hang at startup - before it could even
// answer the MCP initialize handshake. With Lazy, a second process starts
// normally and only its scratch_* calls report ErrBusy, until the other
// process releases the file.
type Lazy struct {
	path    string
	timeout time.Duration

	mu    sync.Mutex
	store *Store
}

// NewLazy returns a Lazy for path that waits at most timeout for the file
// lock on each open attempt.
func NewLazy(path string, timeout time.Duration) *Lazy {
	return &Lazy{path: path, timeout: timeout}
}

// Path returns the store's file path.
func (l *Lazy) Path() string {
	return l.path
}

// Get returns the open Store, opening it first if needed. While another
// process holds the file, it returns an error wrapping ErrBusy.
func (l *Lazy) Get() (*Store, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.store != nil {
		return l.store, nil
	}
	s, err := OpenWithOptions(l.path, &bolt.Options{Timeout: l.timeout})
	if errors.Is(err, bolterrors.ErrTimeout) {
		return nil, fmt.Errorf("%w (%s) - retry once it exits, or pass -store-path to use a separate file", ErrBusy, l.path)
	}
	if err != nil {
		return nil, err
	}
	l.store = s
	return s, nil
}

// Close releases the store's file, if it was ever opened.
func (l *Lazy) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.store == nil {
		return nil
	}
	err := l.store.Close()
	l.store = nil
	return err
}
