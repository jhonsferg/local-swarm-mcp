// Package authmw provides a minimal bearer-token auth middleware for
// local-swarm-mcp's HTTP transport - relevant once the server runs on a
// remote machine (e.g. a GPU box on the same network) rather than being
// spawned as a local stdio subprocess, where anyone who can reach the port
// could otherwise spawn tasks on it.
package authmw

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireBearer wraps next, rejecting any request whose Authorization
// header doesn't present the expected bearer token via constant-time
// comparison.
func RequireBearer(next http.Handler, key string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, prefix)
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) != 1 {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
