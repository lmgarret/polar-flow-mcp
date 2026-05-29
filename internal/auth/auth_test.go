package auth_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lmgarret/polar-flow-mcp/internal/auth"
)

// mockIssuer is an OIDC issuer test double: it serves discovery + introspection.
// active reports whether a presented token is "valid"; the returned claims are
// fixed so tests can exercise the pins.
type mockIssuer struct {
	server   *httptest.Server
	active   bool
	clientID string
	subject  string
	audience []string
	groups   []string
	// lastBasicUser/Pass capture the introspection client credentials sent.
	lastBasicUser string
	lastBasicPass string
}

func newMockIssuer(t *testing.T) *mockIssuer {
	t.Helper()
	m := &mockIssuer{active: true, clientID: "claude", subject: "uuid-1", audience: []string{"https://polar.example.com/mcp"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"introspection_endpoint": m.server.URL + "/introspect",
		})
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, r *http.Request) {
		m.lastBasicUser, m.lastBasicPass, _ = r.BasicAuth()
		_ = r.ParseForm()
		resp := map[string]any{"active": m.active}
		if m.active {
			resp["client_id"] = m.clientID
			resp["sub"] = m.subject
			resp["aud"] = m.audience
			if len(m.groups) > 0 {
				resp["groups"] = m.groups
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func baseConfig(m *mockIssuer) auth.Config {
	return auth.Config{
		Issuer:                    m.server.URL,
		Resource:                  "https://polar.example.com/mcp",
		IntrospectionClientID:     "rs-client",
		IntrospectionClientSecret: "rs-secret",
	}
}

// okHandler is the protected handler; it records whether it was reached.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func do(t *testing.T, h http.Handler, authHeader, origin string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestMiddleware_MissingToken401WithChallenge(t *testing.T) {
	m := newMockIssuer(t)
	a := auth.New(baseConfig(m))
	var reached bool
	resp := do(t, a.Middleware(okHandler(&reached)), "", "")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	if reached {
		t.Error("protected handler should not be reached")
	}
	wantFrag := "resource_metadata="
	if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, wantFrag) {
		t.Errorf("WWW-Authenticate should contain %q, got %q", wantFrag, got)
	}
	if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, "/.well-known/oauth-protected-resource") {
		t.Errorf("WWW-Authenticate should point at metadata path, got %q", got)
	}
}

func TestMiddleware_ValidTokenPasses(t *testing.T) {
	m := newMockIssuer(t)
	a := auth.New(baseConfig(m))
	var reached bool
	resp := do(t, a.Middleware(okHandler(&reached)), "Bearer abc", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if !reached {
		t.Error("protected handler should be reached")
	}
	if m.lastBasicUser != "rs-client" || m.lastBasicPass != "rs-secret" {
		t.Errorf("introspection basic auth: got %q/%q", m.lastBasicUser, m.lastBasicPass)
	}
}

func TestMiddleware_InactiveToken401(t *testing.T) {
	m := newMockIssuer(t)
	m.active = false
	a := auth.New(baseConfig(m))
	var reached bool
	resp := do(t, a.Middleware(okHandler(&reached)), "Bearer abc", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	if reached {
		t.Error("handler should not be reached for inactive token")
	}
}

func TestMiddleware_PinMismatch403(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(auth.Config) auth.Config
	}{
		{"client", func(c auth.Config) auth.Config { c.AllowedClientIDs = []string{"other"}; return c }},
		{"subject", func(c auth.Config) auth.Config { c.AllowedSubjects = []string{"other"}; return c }},
		{"audience", func(c auth.Config) auth.Config { c.AllowedAudiences = []string{"https://other"}; return c }},
		{"group", func(c auth.Config) auth.Config { c.AllowedGroups = []string{"admins"}; return c }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockIssuer(t)
			a := auth.New(tc.cfg(baseConfig(m)))
			var reached bool
			resp := do(t, a.Middleware(okHandler(&reached)), "Bearer abc", "")
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("want 403, got %d", resp.StatusCode)
			}
			if reached {
				t.Error("handler should not be reached on pin mismatch")
			}
		})
	}
}

func TestMiddleware_PinMatchPasses(t *testing.T) {
	m := newMockIssuer(t)
	m.groups = []string{"polar-users"}
	cfg := baseConfig(m)
	cfg.AllowedClientIDs = []string{"claude"}
	cfg.AllowedSubjects = []string{"uuid-1"}
	cfg.AllowedAudiences = []string{"https://polar.example.com/mcp"}
	cfg.AllowedGroups = []string{"polar-users"}
	a := auth.New(cfg)
	var reached bool
	resp := do(t, a.Middleware(okHandler(&reached)), "Bearer abc", "")
	if resp.StatusCode != http.StatusOK || !reached {
		t.Fatalf("want 200 + handler reached, got %d reached=%v", resp.StatusCode, reached)
	}
}

func TestOriginGuard(t *testing.T) {
	m := newMockIssuer(t)
	cfg := baseConfig(m)
	cfg.AllowedOrigins = []string{"https://claude.ai"}
	a := auth.New(cfg)

	// Wrap a plain handler so we isolate Origin behaviour from token checks.
	plain := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	guard := a.OriginGuard(plain)

	if resp := do(t, guard, "", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("no Origin should be allowed, got %d", resp.StatusCode)
	}
	if resp := do(t, guard, "", "https://claude.ai"); resp.StatusCode != http.StatusOK {
		t.Errorf("allowed Origin should pass, got %d", resp.StatusCode)
	}
	if resp := do(t, guard, "", "https://evil.example"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("disallowed Origin should be 403, got %d", resp.StatusCode)
	}
}

func TestOriginGuard_NoopWhenUnset(t *testing.T) {
	m := newMockIssuer(t)
	a := auth.New(baseConfig(m)) // no AllowedOrigins
	plain := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	if resp := do(t, a.OriginGuard(plain), "", "https://evil.example"); resp.StatusCode != http.StatusOK {
		t.Errorf("Origin guard should be a no-op when unset, got %d", resp.StatusCode)
	}
}

func TestMetadataHandler(t *testing.T) {
	m := newMockIssuer(t)
	a := auth.New(baseConfig(m))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, auth.MetadataPath, nil)
	rec := httptest.NewRecorder()
	a.MetadataHandler().ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var doc struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("invalid metadata JSON: %v", err)
	}
	if doc.Resource != "https://polar.example.com/mcp" {
		t.Errorf("resource: got %q", doc.Resource)
	}
	if len(doc.AuthorizationServers) != 1 || doc.AuthorizationServers[0] != m.server.URL {
		t.Errorf("authorization_servers: got %v", doc.AuthorizationServers)
	}
}

// TestMetadataURLForm guards the WWW-Authenticate metadata URL derivation.
func TestMetadataURLForm(t *testing.T) {
	m := newMockIssuer(t)
	a := auth.New(baseConfig(m))
	resp := do(t, a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})), "", "")
	hdr := resp.Header.Get("WWW-Authenticate")
	// Extract the quoted URL and ensure it parses and sits at the host root.
	start := strings.Index(hdr, `"`)
	end := strings.LastIndex(hdr, `"`)
	if start < 0 || end <= start {
		t.Fatalf("no quoted URL in %q", hdr)
	}
	u, err := url.Parse(hdr[start+1 : end])
	if err != nil {
		t.Fatalf("metadata URL does not parse: %v", err)
	}
	if u.Path != auth.MetadataPath {
		t.Errorf("metadata URL path: want %q, got %q", auth.MetadataPath, u.Path)
	}
	if u.Host != "polar.example.com" {
		t.Errorf("metadata URL host: want polar.example.com, got %q", u.Host)
	}
}
