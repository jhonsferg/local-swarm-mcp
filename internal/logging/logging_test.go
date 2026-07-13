package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHub_RecentReturnsWrittenLinesInOrder(t *testing.T) {
	h := NewHub(3)
	_, _ = h.Write([]byte("a"))
	_, _ = h.Write([]byte("b"))
	_, _ = h.Write([]byte("c"))

	got := h.Recent()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Recent() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Recent()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHub_RecentWrapsWhenOverCapacity(t *testing.T) {
	h := NewHub(2)
	_, _ = h.Write([]byte("a"))
	_, _ = h.Write([]byte("b"))
	_, _ = h.Write([]byte("c")) // evicts "a"

	got := h.Recent()
	want := []string{"b", "c"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Recent() = %+v, want %+v", got, want)
	}
}

func TestHub_SubscribeReceivesNewWrites(t *testing.T) {
	h := NewHub(10)
	ch, cancel := h.Subscribe()
	defer cancel()

	_, _ = h.Write([]byte("hello"))

	select {
	case line := <-ch:
		if line != "hello" {
			t.Fatalf("got %q, want hello", line)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribed line")
	}
}

func TestHub_CancelStopsDelivery(t *testing.T) {
	h := NewHub(10)
	ch, cancel := h.Subscribe()
	cancel()

	_, _ = h.Write([]byte("hello"))

	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestNew_WritesToFileAndHub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	hub := NewHub(10)

	logger, closeFn, err := New(path, hub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = closeFn() }()

	logger.Info("test event", "key", "value")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "test event") {
		t.Fatalf("log file content = %q, want it to contain the log message", data)
	}

	recent := hub.Recent()
	if len(recent) != 1 || !strings.Contains(recent[0], "test event") {
		t.Fatalf("hub.Recent() = %+v, want the log line", recent)
	}
}

func TestNew_RotatesOversizedLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	if err := os.WriteFile(path, make([]byte, maxRotateSize+1), 0o644); err != nil {
		t.Fatalf("seed oversized file: %v", err)
	}

	_, closeFn, err := New(path, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = closeFn() }()

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated file %s.1 should exist: %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat new log file: %v", err)
	}
	if info.Size() >= maxRotateSize {
		t.Fatalf("new log file should start fresh, size = %d", info.Size())
	}
}

func TestNew_NilHubIsFine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	logger, closeFn, err := New(path, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = closeFn() }()
	logger.Info("no hub, still works")
	_ = slog.Default() // sanity: package compiles/links against log/slog as expected
}
