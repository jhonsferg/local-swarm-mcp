package authmw

import (
	"net/http"
	"net/http/httptest"
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
