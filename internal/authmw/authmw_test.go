package authmw

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireBearer(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireBearer(inner, "secret")

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic secret", http.StatusUnauthorized},
		{"wrong token", "Bearer wrong", http.StatusUnauthorized},
		{"correct token", "Bearer secret", http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if c.authHeader != "" {
				req.Header.Set("Authorization", c.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}
		})
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuth_DisabledPassesThrough(t *testing.T) {
	a := New("")
	rec := httptest.NewRecorder()
	a.Require(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/hosts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with auth disabled", rec.Code)
	}
}

func TestAuth_LoginSetsSessionCookieThatRequireAccepts(t *testing.T) {
	a := New("secret")

	login := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(`{"token":"`+token+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		a.LoginHandler().ServeHTTP(rec, req)
		return rec
	}

	if rec := login("wrong"); rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("wrong token: status = %d, cookies = %v", rec.Code, rec.Result().Cookies())
	}

	rec := login("secret")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want 204", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookie {
		t.Fatalf("cookies = %v, want one %s cookie", cookies, SessionCookie)
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v, want HttpOnly + SameSite=Strict", c)
	}
	if c.Value == "secret" || strings.Contains(c.Value, "secret") {
		t.Fatal("session cookie must not carry the API key itself")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/hosts", nil)
	req.AddCookie(c)
	rec = httptest.NewRecorder()
	a.Require(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request with session cookie: status = %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/hosts", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "forged"})
	rec = httptest.NewRecorder()
	a.Require(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie: status = %d, want 401", rec.Code)
	}
}

func TestAuth_LoginRequiresJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(`{"token":"secret"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	New("secret").LoginHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

func TestGuardBrowser(t *testing.T) {
	loopback := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8090}
	lan := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 8090}

	cases := []struct {
		name   string
		local  net.Addr
		host   string
		origin string
		want   int
	}{
		{"same-origin dashboard", loopback, "localhost:8090", "http://localhost:8090", http.StatusOK},
		{"CLI or MCP client, no Origin", loopback, "127.0.0.1:8090", "", http.StatusOK},
		{"cross-site page", loopback, "localhost:8090", "https://evil.example", http.StatusForbidden},
		{"opaque origin", loopback, "localhost:8090", "null", http.StatusForbidden},
		{"DNS rebinding", loopback, "evil.example:8090", "http://evil.example:8090", http.StatusForbidden},
		{"LAN request by name", lan, "gpu-box:8090", "", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/admin/register-host", nil)
			req.Host = c.host
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, c.local))
			rec := httptest.NewRecorder()
			GuardBrowser(okHandler()).ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8090": true,
		"localhost:8090": true,
		"[::1]:8090":     true,
		":8090":          false,
		"0.0.0.0:8090":   false,
		"192.168.1.10:1": false,
	}
	for addr, want := range cases {
		if got := IsLoopbackAddr(addr); got != want {
			t.Errorf("IsLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestUnauthorized_SendsBearerChallenge(t *testing.T) {
	rec := httptest.NewRecorder()
	New("secret").Require(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer ") {
		t.Fatalf("WWW-Authenticate = %q, want a Bearer challenge", got)
	}
}
