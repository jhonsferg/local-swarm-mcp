package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

func fakeChatServer(t *testing.T, reply string, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": reply}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testRegistry(baseURL string) *backend.Registry {
	return backend.NewRegistry([]config.Backend{{Name: "test", BaseURL: baseURL, Model: "m"}})
}

func TestTaskRegistry_SpawnAndWait(t *testing.T) {
	srv := fakeChatServer(t, "hello", 0)
	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))

	id, err := tr.Spawn("", "", "hi", 64, 0.1)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if info.Status != TaskCompleted || info.Result != "hello" {
		t.Fatalf("unexpected task info: %+v", info)
	}
}

func TestTaskRegistry_UnknownBackend(t *testing.T) {
	tr := NewTaskRegistry(backend.NewClient(), backend.NewRegistry(nil))
	if _, err := tr.Spawn("nope", "", "hi", 64, 0.1); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

func TestTaskRegistry_Cancel(t *testing.T) {
	srv := fakeChatServer(t, "hello", 2*time.Second)
	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))

	id, err := tr.Spawn("", "", "hi", 64, 0.1)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Give the goroutine a moment to move past "pending".
	time.Sleep(50 * time.Millisecond)

	if err := tr.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	info, err := tr.Wait(context.Background(), id, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait after cancel: %v", err)
	}
	if info.Status != TaskCancelled {
		t.Fatalf("expected TaskCancelled, got %+v", info)
	}
}

func TestTaskRegistry_ListAndStatus(t *testing.T) {
	srv := fakeChatServer(t, "ok", 0)
	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))

	id, err := tr.Spawn("", "", "hi", 64, 0.1)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := tr.Wait(context.Background(), id, 5*time.Second); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	list := tr.List()
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("unexpected list: %+v", list)
	}

	if _, err := tr.Status("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown task ID")
	}
}
