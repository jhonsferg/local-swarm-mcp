package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/jhonsferg/local-swarm-mcp/internal/backend"
)

type session struct {
	id      string
	backend string
	system  string
	mu      sync.Mutex
	history []backend.ChatMessage
}

// SessionInfo is a summary of a session for listing purposes.
type SessionInfo struct {
	ID           string `json:"id"`
	Backend      string `json:"backend"`
	MessageCount int    `json:"message_count"`
}

// SessionRegistry tracks multi-turn conversations against a backend,
// keeping each session's history so follow-up messages carry context -
// analogous to resuming a named agent rather than starting fresh each call.
type SessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]*session
	client   *backend.Client
	registry *backend.Registry
}

// NewSessionRegistry builds an empty SessionRegistry.
func NewSessionRegistry(client *backend.Client, registry *backend.Registry) *SessionRegistry {
	return &SessionRegistry{
		sessions: make(map[string]*session),
		client:   client,
		registry: registry,
	}
}

// Create opens a new session against backendName (or the default backend,
// if empty) with an optional system prompt, and returns its ID.
func (sr *SessionRegistry) Create(backendName, systemPrompt string) (string, error) {
	cfg, err := sr.registry.Get(backendName)
	if err != nil {
		return "", err
	}

	id := newID()
	s := &session{id: id, backend: cfg.Name, system: systemPrompt}

	sr.mu.Lock()
	sr.sessions[id] = s
	sr.mu.Unlock()

	return id, nil
}

func (sr *SessionRegistry) get(id string) (*session, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	s, ok := sr.sessions[id]
	if !ok {
		return nil, fmt.Errorf("unknown session %q", id)
	}
	return s, nil
}

// Send appends message to the session's history, sends the full transcript
// to the backend, and appends the reply before returning it.
func (sr *SessionRegistry) Send(ctx context.Context, id, message string, maxTokens int, temperature, topP float64) (string, error) {
	s, err := sr.get(id)
	if err != nil {
		return "", err
	}

	cfg, err := sr.registry.Get(s.backend)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var messages []backend.ChatMessage
	if s.system != "" {
		messages = append(messages, backend.ChatMessage{Role: "system", Content: s.system})
	}
	messages = append(messages, s.history...)
	messages = append(messages, backend.ChatMessage{Role: "user", Content: message})

	reply, err := sr.client.Complete(ctx, cfg, messages, maxTokens, temperature, topP)
	if err != nil {
		return "", err
	}

	s.history = append(s.history,
		backend.ChatMessage{Role: "user", Content: message},
		backend.ChatMessage{Role: "assistant", Content: reply},
	)
	return reply, nil
}

// History returns the full message history of a session.
func (sr *SessionRegistry) History(id string) ([]backend.ChatMessage, error) {
	s, err := sr.get(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]backend.ChatMessage, len(s.history))
	copy(out, s.history)
	return out, nil
}

// Close discards a session.
func (sr *SessionRegistry) Close(id string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if _, ok := sr.sessions[id]; !ok {
		return fmt.Errorf("unknown session %q", id)
	}
	delete(sr.sessions, id)
	return nil
}

// List returns a summary of every open session.
func (sr *SessionRegistry) List() []SessionInfo {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	out := make([]SessionInfo, 0, len(sr.sessions))
	for _, s := range sr.sessions {
		s.mu.Lock()
		out = append(out, SessionInfo{ID: s.id, Backend: s.backend, MessageCount: len(s.history)})
		s.mu.Unlock()
	}
	return out
}
