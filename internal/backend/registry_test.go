package backend

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

func TestRegistry_GetDefaultsToFirstStaticBackend(t *testing.T) {
	r := NewRegistry([]config.Backend{
		{Name: "llama", Model: "llama3.1:8b"},
		{Name: "qwen", Model: "qwen2.5-coder:1.5b"},
	})

	got, err := r.Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if got.Name != "llama" {
		t.Fatalf("Get(\"\").Name = %q, want llama", got.Name)
	}
}

func TestRegistry_GetUnknownName(t *testing.T) {
	r := NewRegistry(nil)
	if _, err := r.Get("missing"); err == nil {
		t.Fatal("Get(missing) should error")
	}
}

func TestRegistry_GetNoBackendsConfigured(t *testing.T) {
	r := NewRegistry(nil)
	if _, err := r.Get(""); err == nil {
		t.Fatal("Get(\"\") with no backends should error")
	}
}

func TestRegistry_PutAddsDynamicEntryWithoutAffectingDefault(t *testing.T) {
	r := NewRegistry([]config.Backend{{Name: "llama", Model: "llama3.1:8b"}})

	r.Put(config.Backend{Name: "rx9070/gemma4:12b", Model: "gemma4:12b", BaseURL: "http://192.168.18.29:11434/v1"})

	got, err := r.Get("rx9070/gemma4:12b")
	if err != nil {
		t.Fatalf("Get(rx9070/gemma4:12b): %v", err)
	}
	if got.Model != "gemma4:12b" {
		t.Fatalf("got.Model = %q, want gemma4:12b", got.Model)
	}

	// The static default is unaffected by a dynamic Put.
	def, err := r.Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if def.Name != "llama" {
		t.Fatalf("default = %q, want llama", def.Name)
	}
}

func TestRegistry_RemoveDeletesDynamicEntry(t *testing.T) {
	r := NewRegistry(nil)
	r.Put(config.Backend{Name: "rx9070/gemma4:12b"})
	r.Remove("rx9070/gemma4:12b")

	if _, err := r.Get("rx9070/gemma4:12b"); err == nil {
		t.Fatal("Get should error after Remove")
	}
}

func TestRegistry_ListIncludesStaticAndDynamicEntries(t *testing.T) {
	r := NewRegistry([]config.Backend{{Name: "llama"}, {Name: "qwen"}})
	r.Put(config.Backend{Name: "rx9070/gemma4:12b"})

	names := make(map[string]bool)
	for _, b := range r.List() {
		names[b.Name] = true
	}
	for _, want := range []string{"llama", "qwen", "rx9070/gemma4:12b"} {
		if !names[want] {
			t.Fatalf("List() = %+v, missing %q", r.List(), want)
		}
	}
}

func TestRegistry_ConcurrentPutAndGet(t *testing.T) {
	r := NewRegistry([]config.Backend{{Name: "llama"}})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			r.Put(config.Backend{Name: "dynamic"})
		}(i)
		go func() {
			defer wg.Done()
			_, _ = r.Get("llama")
			_ = r.List()
		}()
	}
	wg.Wait()
}

func TestRegistry_GetDefaultsToFirstDynamicBackendWithNoStaticOnes(t *testing.T) {
	// A host-discovery-only setup (zero static backends) must still resolve
	// an omitted backend name instead of failing with "no backends
	// configured" while discovered backends exist.
	r := NewRegistry(nil)
	r.Put(config.Backend{Name: "rx9070/qwen3.5:latest"})
	r.Put(config.Backend{Name: "rx9070/gemma4:12b"})
	r.Put(config.Backend{Name: "laptop/llama3.1:8b"})

	got, err := r.Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if got.Name != "laptop/llama3.1:8b" {
		t.Fatalf("Get(\"\").Name = %q, want laptop/llama3.1:8b (first in name order)", got.Name)
	}
}

func TestRegistry_ListOrdersStaticThenDynamicByName(t *testing.T) {
	r := NewRegistry([]config.Backend{{Name: "zeta"}, {Name: "alpha"}})
	r.Put(config.Backend{Name: "rx9070/qwen3.5:latest"})
	r.Put(config.Backend{Name: "laptop/llama3.1:8b"})

	var got []string
	for _, b := range r.List() {
		got = append(got, b.Name)
	}
	want := []string{"zeta", "alpha", "laptop/llama3.1:8b", "rx9070/qwen3.5:latest"}
	if len(got) != len(want) {
		t.Fatalf("List() names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List() names = %v, want %v", got, want)
		}
	}

	def, err := r.Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if def.Name != got[0] {
		t.Fatalf("Get(\"\") = %q, want List()[0] = %q", def.Name, got[0])
	}
}

func TestRegistry_InfosRedactAPIKey(t *testing.T) {
	r := NewRegistry([]config.Backend{{Name: "remote", BaseURL: "https://api.example/v1", Model: "m", APIKey: "super-secret"}})
	infos := r.Infos()
	if len(infos) != 1 || !infos[0].HasAPIKey || infos[0].Name != "remote" {
		t.Fatalf("Infos() = %+v", infos)
	}
	data, _ := json.Marshal(infos)
	if strings.Contains(string(data), "super-secret") {
		t.Fatalf("Infos() JSON leaks the key: %s", data)
	}
}
