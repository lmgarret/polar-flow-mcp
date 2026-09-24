package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "s3cret-key-that-is-long-enough"

// apiKeyHandler wraps a handler that records whether it was reached.
func apiKeyHandler(t *testing.T, reached *bool) http.Handler {
	t.Helper()
	g := NewAPIKeyGuard(testKey, nil)
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	}))
}

func TestAPIKeyGuard(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"correct key", "Bearer " + testKey, http.StatusOK},
		{"lowercase scheme", "bearer " + testKey, http.StatusOK},
		{"no header", "", http.StatusUnauthorized},
		{"wrong key", "Bearer nope-nope-nope-nope-nope-nope", http.StatusUnauthorized},
		{"prefix of key", "Bearer " + testKey[:10], http.StatusUnauthorized},
		{"key with suffix", "Bearer " + testKey + "x", http.StatusUnauthorized},
		{"wrong scheme", "Basic " + testKey, http.StatusUnauthorized},
		{"bare key, no scheme", testKey, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reached bool
			h := apiKeyHandler(t, &reached)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if want := tc.wantStatus == http.StatusOK; reached != want {
				t.Errorf("handler reached = %v, want %v", reached, want)
			}
			if tc.wantStatus == http.StatusUnauthorized {
				if c := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(c, "Bearer ") {
					t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", c)
				}
			}
		})
	}
}

// TestAPIKeyGuardDoesNotLeakKey guards against the key ending up in a response
// body — the 401 text is the only thing an unauthenticated caller ever sees.
func TestAPIKeyGuardDoesNotLeakKey(t *testing.T) {
	var reached bool
	h := apiKeyHandler(t, &reached)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), testKey) {
		t.Error("401 body leaked the configured API key")
	}
}
