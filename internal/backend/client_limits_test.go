package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jhonsferg/local-swarm-mcp/internal/config"
)

func TestComplete_SendsExplicitZeroTemperature(t *testing.T) {
	// Reproduces a confirmed bug: temperature=0 was dropped by omitempty,
	// so the backend silently used its own default instead.
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	b := config.Backend{Name: "t", BaseURL: srv.URL, Model: "m"}
	if _, err := NewClient().Complete(context.Background(), b, []ChatMessage{{Role: "user", Content: "hi"}}, 16, 0, 0); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	temp, ok := body["temperature"]
	if !ok || temp != float64(0) {
		t.Fatalf("request body temperature = %v (present=%v), want an explicit 0", temp, ok)
	}
	if _, ok := body["top_p"]; ok {
		t.Fatalf("top_p=0 means \"backend default\" and should be omitted, body = %v", body)
	}
}

func TestComplete_CapsConcurrentRequestsPerBackend(t *testing.T) {
	var inFlight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		inFlight.Add(-1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := NewClientWithOptions(ClientOptions{MaxConcurrentPerBackend: 2})
	// Two "<host>/<model>" backends on the same host share one limit.
	backends := []config.Backend{
		{Name: "h/a", BaseURL: srv.URL, Model: "a"},
		{Name: "h/b", BaseURL: srv.URL, Model: "b"},
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Complete(context.Background(), backends[i%2], []ChatMessage{{Role: "user", Content: "hi"}}, 16, 0.2, 0); err != nil {
				t.Errorf("Complete: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if got := peak.Load(); got > 2 {
		t.Fatalf("peak concurrent requests = %d, want at most 2", got)
	}
}

func TestComplete_WaitingForSlotHonorsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()
	defer close(release)

	c := NewClientWithOptions(ClientOptions{MaxConcurrentPerBackend: 1})
	b := config.Backend{Name: "t", BaseURL: srv.URL, Model: "m"}
	msgs := []ChatMessage{{Role: "user", Content: "hi"}}

	go func() { _, _ = c.Complete(context.Background(), b, msgs, 16, 0.2, 0) }() // holds the only slot
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.Complete(ctx, b, msgs, 16, 0.2, 0)
	if err == nil || !strings.Contains(err.Error(), "free slot") {
		t.Fatalf("Complete while the slot is held = %v, want a context error waiting for a slot", err)
	}
}

func TestComplete_TimeoutOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := NewClientWithOptions(ClientOptions{Timeout: 50 * time.Millisecond})
	b := config.Backend{Name: "t", BaseURL: srv.URL, Model: "m"}
	if _, err := c.Complete(context.Background(), b, []ChatMessage{{Role: "user", Content: "hi"}}, 16, 0.2, 0); err == nil {
		t.Fatal("expected a timeout error")
	}
}
