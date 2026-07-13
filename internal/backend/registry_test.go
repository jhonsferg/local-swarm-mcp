package backend

import (
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
