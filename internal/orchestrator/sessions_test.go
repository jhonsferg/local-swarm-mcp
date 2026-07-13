package orchestrator

import (
	"context"
	"testing"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
)

func TestSessionRegistry_CreateSendHistoryClose(t *testing.T) {
	srv := fakeChatServer(t, "reply", 0)
	sr := NewSessionRegistry(backend.NewClient(), testRegistry(srv.URL))

	id, err := sr.Create("", "be terse")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	reply, err := sr.Send(context.Background(), id, "hi", 64, 0.1)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if reply != "reply" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	history, err := sr.History(id)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 || history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("unexpected history: %+v", history)
	}

	list := sr.List()
	if len(list) != 1 || list[0].MessageCount != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}

	if err := sr.Close(id); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := sr.History(id); err == nil {
		t.Fatal("expected an error after closing the session")
	}
}

func TestSessionRegistry_UnknownSession(t *testing.T) {
	sr := NewSessionRegistry(backend.NewClient(), backend.NewRegistry(nil))
	if _, err := sr.Send(context.Background(), "nope", "hi", 64, 0.1); err == nil {
		t.Fatal("expected an error for an unknown session")
	}
}
