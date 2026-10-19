package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
)

func spawnAndWait(t *testing.T, tr *TaskRegistry) string {
	t.Helper()
	id, err := tr.Spawn("", "", "hi", 16, 0.1, 0)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := tr.Wait(context.Background(), id, 5*time.Second); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return id
}

func TestTaskRegistry_EvictsOldestFinishedBeyondMax(t *testing.T) {
	srv := fakeChatServer(t, "ok", 0)
	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	tr.MaxFinished = 2

	first := spawnAndWait(t, tr)
	spawnAndWait(t, tr)
	spawnAndWait(t, tr)
	spawnAndWait(t, tr) // pruning runs on spawn: 3 finished > 2, oldest goes

	if _, err := tr.Status(first); err == nil {
		t.Fatal("the oldest finished task should have been evicted")
	}
	if n := len(tr.List()); n > 3 {
		t.Fatalf("len(List()) = %d, want finished tasks bounded by MaxFinished", n)
	}
}

func TestTaskRegistry_EvictsFinishedAfterTTLButNeverRunning(t *testing.T) {
	srv := fakeChatServer(t, "ok", 0)
	slow := fakeChatServer(t, "slow", 2*time.Second)
	tr := NewTaskRegistry(backend.NewClient(), testRegistry(srv.URL))
	tr.FinishedTTL = 10 * time.Millisecond

	finished := spawnAndWait(t, tr)

	tr.registry = testRegistry(slow.URL)
	running, err := tr.Spawn("", "", "hi", 16, 0.1, 0)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := tr.Spawn("", "", "hi", 16, 0.1, 0); err != nil { // triggers pruning
		t.Fatalf("Spawn: %v", err)
	}

	if _, err := tr.Status(finished); err == nil {
		t.Fatal("a finished task past FinishedTTL should have been evicted")
	}
	if _, err := tr.Status(running); err != nil {
		t.Fatalf("a running task must never be evicted: %v", err)
	}
}

func TestSessionRegistry_CapsHistoryInPairs(t *testing.T) {
	srv := fakeChatServer(t, "reply", 0)
	sr := NewSessionRegistry(backend.NewClient(), testRegistry(srv.URL))
	sr.MaxHistoryMessages = 4

	id, err := sr.Create("", "be terse")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, msg := range []string{"m1", "m2", "m3"} {
		if _, err := sr.Send(context.Background(), id, msg, 16, 0.1, 0); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	history, err := sr.History(id)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("len(history) = %d, want 4", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "m2" {
		t.Fatalf("history should start at the oldest kept user message m2, got %+v", history[0])
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("truncate(short) = %q", got)
	}
	long := strings.Repeat("é", 10) // 2 bytes each
	got := truncate(long, 5)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate must cut on a rune boundary, got %q", got)
	}
	if !strings.HasPrefix(got, "éé\n...[truncated 16 bytes]") {
		t.Fatalf("truncate(long) = %q", got)
	}
}
