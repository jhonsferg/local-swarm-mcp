// Package logging provides local-swarm-mcp's structured logging: every
// significant server event (election outcome, host registered/polled,
// downstream MCP server connected/disconnected, HTTP admin actions) goes
// through a single slog.Logger that fans out to stderr, a log file, and an
// in-memory hub the embedded web UI streams from live via Server-Sent
// Events - so "what is the daemon doing right now" is always answerable
// without attaching a debugger or grepping a file by hand.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// maxRotateSize is the size threshold at which the log file is rotated
// (renamed to .1, overwriting any previous .1) on startup. Simple
// size-on-open rotation, not a full rotating-writer - good enough to keep
// a long-lived daemon's log from growing unbounded between restarts.
const maxRotateSize = 10 * 1024 * 1024 // 10 MiB

// Hub fans out log lines to live subscribers (the embedded UI's SSE log
// tail) and keeps a bounded ring buffer of the most recent lines so a
// client connecting late still sees recent history.
type Hub struct {
	mu   sync.Mutex
	buf  []string
	cap  int
	next int
	full bool
	subs map[chan string]struct{}
}

// NewHub creates a Hub retaining up to capacity recent log lines.
func NewHub(capacity int) *Hub {
	return &Hub{
		buf:  make([]string, capacity),
		cap:  capacity,
		subs: make(map[chan string]struct{}),
	}
}

// Write implements io.Writer so a Hub can be used directly as one of
// several io.MultiWriter destinations for the slog handler.
func (h *Hub) Write(p []byte) (int, error) {
	line := string(p)
	h.mu.Lock()
	if h.cap > 0 {
		h.buf[h.next] = line
		h.next = (h.next + 1) % h.cap
		if h.next == 0 {
			h.full = true
		}
	}
	for ch := range h.subs {
		select {
		case ch <- line:
		default: // slow subscriber: drop rather than block logging
		}
	}
	h.mu.Unlock()
	return len(p), nil
}

// Recent returns the retained log lines, oldest first.
func (h *Hub) Recent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.full {
		out := make([]string, h.next)
		copy(out, h.buf[:h.next])
		return out
	}
	out := make([]string, h.cap)
	copy(out, h.buf[h.next:])
	copy(out[h.cap-h.next:], h.buf[:h.next])
	return out
}

// Subscribe registers a channel that receives every log line written from
// this point on. Call the returned cancel func to unsubscribe (e.g. when
// an SSE client disconnects).
func (h *Hub) Subscribe() (ch chan string, cancel func()) {
	ch = make(chan string, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
		close(ch)
	}
}

// New builds the daemon's logger: JSON-structured records fan out to
// stderr, a log file at logPath (rotated on open if it's grown past
// maxRotateSize), and hub (for the embedded UI's live tail). Returns the
// logger and a close func to release the file handle on shutdown.
func New(logPath string, hub *Hub) (*slog.Logger, func() error, error) {
	if dir := filepath.Dir(logPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
	}
	rotateIfLarge(logPath)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	writers := []io.Writer{os.Stderr, f}
	if hub != nil {
		writers = append(writers, hub)
	}

	handler := slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), f.Close, nil
}

func rotateIfLarge(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxRotateSize {
		return
	}
	_ = os.Rename(path, path+".1")
}

// DefaultLogPath returns the daemon's log file location when none is
// overridden.
func DefaultLogPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "local-swarm-mcp.log"
	}
	return filepath.Join(dir, "local-swarm-mcp", "daemon.log")
}
