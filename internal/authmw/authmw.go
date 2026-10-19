// Package authmw provides local-swarm-mcp's HTTP access control: bearer-token
// auth (plus a dashboard session cookie derived from the same token, since
// a browser's EventSource and page loads can't send an Authorization
// header), and a browser guard against cross-origin requests and DNS
// rebinding - relevant once the server runs as a -transport http daemon
// rather than being spawned as a local stdio subprocess, where anyone who
// can reach the port could otherwise spawn tasks on it.
package authmw

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// SessionCookie is the name of the dashboard session cookie.
const SessionCookie = "lsm_session"

// RequireBearer wraps next, rejecting any request whose Authorization
// header doesn't present the expected bearer token via constant-time
// comparison.
func RequireBearer(next http.Handler, key string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearer(r, key) {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validBearer(r *http.Request, key string) bool {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := strings.TrimPrefix(auth, prefix)
	return subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1
}

// Auth is the daemon's access policy for one API key. A zero key means
// auth is disabled (-insecure-no-auth): Require passes everything through.
type Auth struct {
	key     string
	session string
}

// New builds an Auth for key ("" disables auth).
func New(key string) *Auth {
	a := &Auth{key: key}
	if key != "" {
		// The cookie carries a value derived from the key rather than the
		// key itself, so a leaked cookie can't be replayed as a bearer
		// token against /mcp from outside a browser.
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte("local-swarm-mcp dashboard session v1"))
		a.session = hex.EncodeToString(mac.Sum(nil))
	}
	return a
}

// Enabled reports whether requests must authenticate.
func (a *Auth) Enabled() bool {
	return a.key != ""
}

// Require wraps next, rejecting requests that present neither the bearer
// token nor a valid dashboard session cookie.
func (a *Auth) Require(next http.Handler) http.Handler {
	if !a.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearer(r, a.key) && !a.validSession(r) {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Auth) validSession(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(a.session)) == 1
}

// LoginHandler serves POST {"token": "..."}: on a correct token it sets
// the HttpOnly, SameSite=Strict session cookie the dashboard uses for
// every later /admin request. With auth disabled it succeeds without one.
func (a *Auth) LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !IsJSON(r) {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		if !a.Enabled() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(req.Token), []byte(a.key)) != 1 {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookie,
			Value:    a.session,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

// LogoutHandler serves POST, clearing the session cookie.
func (a *Auth) LogoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

// IsJSON reports whether r declares an application/json body. Requiring it
// on state-changing endpoints means a cross-site <form> or a "simple"
// fetch (text/plain, no CORS preflight) can't reach them from a browser.
func IsJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// GuardBrowser wraps next, rejecting the two ways a web page the user
// happens to visit could reach a daemon through their browser:
//
//   - DNS rebinding: a request that arrived on a loopback address but
//     names a non-loopback Host (an attacker's domain re-pointed at
//     127.0.0.1) is refused, the same rule mcp-go applies to /mcp.
//   - Cross-origin requests: any request carrying an Origin header that
//     doesn't match its own Host is refused. MCP clients and the CLI send
//     no Origin; the embedded dashboard is same-origin.
func GuardBrowser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && local != nil {
			if isLoopbackHost(local.String()) && !isLoopbackHost(r.Host) {
				http.Error(w, "forbidden: invalid Host header", http.StatusForbidden)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// IsLoopbackAddr reports whether a listen address like "127.0.0.1:8090"
// or "localhost:8090" binds only to loopback. An empty host (":8090")
// binds every interface, so it is not.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	return isLoopbackHost(host)
}

// isLoopbackHost reports whether hostport ("127.0.0.1:8090", "[::1]",
// "localhost") names a loopback host.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// unauthorized answers 401 with the RFC 6750 challenge. MCP clients that
// see a 401 probe for OAuth; the challenge names the static bearer scheme
// this server actually uses (no resource_metadata - there is no OAuth
// authorization server to discover), so they fall back to the configured
// Authorization header instead of an endless discovery attempt.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="local-swarm-mcp"`)
	http.Error(w, "missing or invalid bearer token - send \"Authorization: Bearer <-api-key value>\"", http.StatusUnauthorized)
}
