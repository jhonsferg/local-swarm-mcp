package store

import (
	"path/filepath"
	"testing"
)

func TestStore_SetGetDeleteList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scratch.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if _, ok, err := s.Get("missing"); err != nil || ok {
		t.Fatalf("expected missing key to be absent, got ok=%v err=%v", ok, err)
	}

	if err := s.Set("k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("k2", "v2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	value, ok, err := s.Get("k1")
	if err != nil || !ok || value != "v1" {
		t.Fatalf("Get k1 = %q, %v, %v", value, ok, err)
	}

	keys, err := s.List()
	if err != nil || len(keys) != 2 {
		t.Fatalf("List = %v, %v", keys, err)
	}

	if err := s.Delete("k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := s.Get("k1"); err != nil || ok {
		t.Fatalf("expected k1 to be gone after Delete, got ok=%v err=%v", ok, err)
	}
}

func TestStore_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scratch.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s1.Set("persisted", "yes"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()

	value, ok, err := s2.Get("persisted")
	if err != nil || !ok || value != "yes" {
		t.Fatalf("Get after reopen = %q, %v, %v", value, ok, err)
	}
}
