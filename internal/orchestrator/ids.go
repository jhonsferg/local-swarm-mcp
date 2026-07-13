package orchestrator

import (
	"crypto/rand"
	"encoding/hex"
)

// newID returns a short random hex identifier for a task or session.
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read on a supported platform practically never fails;
		// panicking here would be worse than a slightly-worse-than-random ID.
		return hex.EncodeToString([]byte("fallback"))
	}
	return hex.EncodeToString(b)
}
