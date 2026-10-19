package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLazy_OpensOnFirstGetAndReusesStore(t *testing.T) {
	l := NewLazy(filepath.Join(t.TempDir(), "scratch.db"), 100*time.Millisecond)
	defer func() { _ = l.Close() }()

	s1, err := l.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	s2, err := l.Get()
	if err != nil {
		t.Fatalf("Get (2nd): %v", err)
	}
	if s1 != s2 {
		t.Fatal("Get should reuse the already-open store")
	}
}

func TestLazy_ReportsBusyThenRecoversOnceReleased(t *testing.T) {
	// Reproduces a confirmed bug: a second process opening the same
	// scratch store used to block forever at startup. Lazy must fail fast
	// with ErrBusy instead, and succeed on a later call once the holder
	// releases the file.
	path := filepath.Join(t.TempDir(), "scratch.db")
	holder, err := Open(path)
	if err != nil {
		t.Fatalf("Open (holder): %v", err)
	}

	l := NewLazy(path, 100*time.Millisecond)
	defer func() { _ = l.Close() }()

	start := time.Now()
	_, err = l.Get()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Get while held = %v, want ErrBusy", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Get while held took %s, want it bounded by the lock timeout", elapsed)
	}

	if err := holder.Close(); err != nil {
		t.Fatalf("Close (holder): %v", err)
	}
	s, err := l.Get()
	if err != nil {
		t.Fatalf("Get after release: %v", err)
	}
	if err := s.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
}

func TestLazy_CloseWithoutOpenIsNoop(t *testing.T) {
	l := NewLazy(filepath.Join(t.TempDir(), "scratch.db"), 100*time.Millisecond)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
