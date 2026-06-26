package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/auth"
)

const (
	testIssuer   = "http://polar.test"
	testResource = "http://polar.test/mcp"
	testEmail    = "me@example.com"
	testRedirect = "https://claude.ai/api/mcp/auth_callback"
	// A valid PKCE verifier (43-128 chars from the unreserved set).
	pkceVerifier = "abcdefghijklmnopqrstuvwxyz0123456789-._~ABCDEF"
)

func cidrs(t *testing.T, list ...string) []*net.IPNet {
	t.Helper()
	out := make([]*net.IPNet, 0, len(list))
	for _, c := range list {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("bad CIDR %q: %v", c, err)
		}
		out = append(out, n)
	}
	return out
}

// newAuth builds an Authenticator with a throwaway signing key and the given
// trusted-proxy networks.
func newAuth(t *testing.T, trusted []*net.IPNet) *auth.Authenticator {
	t.Helper()
	a, err := auth.New(auth.Config{
		Issuer:         testIssuer,
		Resource:       testResource,
		AllowedEmails:  []string{testEmail},
		EmailHeader:    "Remote-Email",
		GroupsHeader:   "Remote-Groups",
		TrustedProxies: trusted,
		SigningKeyPath: filepath.Join(t.TempDir(), "key.json"),
		AccessTTL:      time.Hour,
		RefreshTTL:     24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return a
}

func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func pkceChallenge() string {
	sum := sha256.Sum256([]byte(pkceVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// TestFullFlow drives register → authorize → token → /mcp end to end.
func TestFullFlow(t *testing.T) {
	var reached bool
	a := newAuth(t, cidrs(t, "127.0.0.0/8", "::1/128"))
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	mux.Handle("/mcp", a.ProtectedHandler(okHandler(&reached)))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// 1. Dynamic Client Registration.
	clientID := register(t, client, srv.URL, testRedirect)

	// 2. Authorize (browser leg, forward-auth header set, trusted peer).
	authzURL := srv.URL + auth.AuthorizePath + "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {testRedirect},
		"code_challenge":        {pkceChallenge()},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
		"scope":                 {"openid"},
	}.Encode()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, authzURL, nil)
	req.Header.Set("Remote-Email", testEmail)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize: want 302, got %d", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("state") != "xyz" {
		t.Errorf("state not echoed: %q", loc.Query().Get("state"))
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("no code in redirect %q", resp.Header.Get("Location"))
	}

	// 3. Token exchange (PKCE).
	tok := exchange(t, client, srv.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirect},
		"client_id":     {clientID},
		"code_verifier": {pkceVerifier},
	})
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Fatalf("missing tokens: %+v", tok)
	}

	// 4. Call /mcp with the access token.
	if status := callMCP(t, client, srv.URL, tok.AccessToken); status != http.StatusOK {
		t.Fatalf("/mcp with valid token: want 200, got %d", status)
	}
	if !reached {
		t.Error("protected handler not reached with valid token")
	}

	// 5. Refresh.
	tok2 := exchange(t, client, srv.URL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {clientID},
	})
	if tok2.AccessToken == "" {
		t.Fatalf("refresh did not return an access token")
	}
	if status := callMCP(t, client, srv.URL, tok2.AccessToken); status != http.StatusOK {
		t.Fatalf("/mcp with refreshed token: want 200, got %d", status)
	}
}

func TestMCP_NoTokenChallenges(t *testing.T) {
	a := newAuth(t, cidrs(t, "127.0.0.0/8"))
	h := a.ProtectedHandler(okHandler(new(bool)))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	wa := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wa, "resource_metadata=") || !strings.Contains(wa, auth.MetadataPath) {
		t.Errorf("WWW-Authenticate missing metadata pointer: %q", wa)
	}
}

func TestMCP_GarbageTokenRejected(t *testing.T) {
	a := newAuth(t, cidrs(t, "127.0.0.0/8"))
	h := a.ProtectedHandler(okHandler(new(bool)))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for garbage token, got %d", rec.Code)
	}
}

func TestAuthorize_UntrustedPeerNoIdentity(t *testing.T) {
	// Trust only 192.0.2.0/24; httptest.NewRequest's RemoteAddr is 192.0.2.1,
	// so flip the expectation by trusting a different net here.
	a := newAuth(t, cidrs(t, "10.0.0.0/8"))
	clientID := mintClientViaServer(t, a)

	req := authorizeReq(t, clientID)
	req.RemoteAddr = "192.0.2.1:5555" // not in 10.0.0.0/8
	req.Header.Set("Remote-Email", testEmail)
	rec := httptest.NewRecorder()
	a.AuthorizeHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("untrusted peer should get 401, got %d", rec.Code)
	}
}

func TestAuthorize_DisallowedEmailForbidden(t *testing.T) {
	a := newAuth(t, cidrs(t, "192.0.2.0/24"))
	clientID := mintClientViaServer(t, a)

	req := authorizeReq(t, clientID)
	req.Header.Set("Remote-Email", "intruder@example.com")
	rec := httptest.NewRecorder()
	a.AuthorizeHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed email should get 403, got %d", rec.Code)
	}
}

func TestRegister_RejectsBadRedirect(t *testing.T) {
	a := newAuth(t, cidrs(t, "127.0.0.0/8"))
	body, _ := json.Marshal(map[string]any{"redirect_uris": []string{"https://evil.example/cb"}})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, auth.RegisterPath, strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	a.RegisterHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("evil redirect_uri should be 400, got %d", rec.Code)
	}
}

func TestToken_PKCEMismatch(t *testing.T) {
	a := newAuth(t, cidrs(t, "192.0.2.0/24"))
	clientID := mintClientViaServer(t, a)

	// Get a code.
	req := authorizeReq(t, clientID)
	req.Header.Set("Remote-Email", testEmail)
	rec := httptest.NewRecorder()
	a.AuthorizeHandler().ServeHTTP(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("no code issued")
	}

	// Exchange with the WRONG verifier.
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirect},
		"client_id":     {clientID},
		"code_verifier": {"wrong-verifier-wrong-verifier-wrong-verifier"},
	}
	treq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, auth.TokenPath, strings.NewReader(form.Encode()))
	treq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	trec := httptest.NewRecorder()
	a.TokenHandler().ServeHTTP(trec, treq)
	if trec.Code != http.StatusBadRequest {
		t.Fatalf("PKCE mismatch should be 400, got %d", trec.Code)
	}
}

func TestMetadataHandlers(t *testing.T) {
	a := newAuth(t, cidrs(t, "127.0.0.0/8"))

	// Protected-resource metadata points the AS at ourselves.
	rec := httptest.NewRecorder()
	a.MetadataHandler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, auth.MetadataPath, nil))
	var prm struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	decode(t, rec.Body, &prm)
	if prm.Resource != testResource {
		t.Errorf("resource: got %q", prm.Resource)
	}
	if len(prm.AuthorizationServers) != 1 || prm.AuthorizationServers[0] != testIssuer {
		t.Errorf("authorization_servers should be self: got %v", prm.AuthorizationServers)
	}

	// AS metadata advertises our endpoints + S256.
	rec = httptest.NewRecorder()
	a.ASMetadataHandler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, auth.ASMetadataPath, nil))
	var asm struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RegistrationEndpoint  string   `json:"registration_endpoint"`
		CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
	}
	decode(t, rec.Body, &asm)
	if asm.Issuer != testIssuer {
		t.Errorf("issuer: got %q", asm.Issuer)
	}
	if asm.RegistrationEndpoint != testIssuer+auth.RegisterPath {
		t.Errorf("registration_endpoint: got %q", asm.RegistrationEndpoint)
	}
	if len(asm.CodeChallengeMethods) != 1 || asm.CodeChallengeMethods[0] != "S256" {
		t.Errorf("code_challenge_methods: got %v", asm.CodeChallengeMethods)
	}
}

// --- helpers ---

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func register(t *testing.T, c *http.Client, base, redirect string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"redirect_uris": []string{redirect}})
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, base+auth.RegisterPath, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", resp.StatusCode)
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	decode(t, resp.Body, &out)
	if out.ClientID == "" {
		t.Fatal("register: empty client_id")
	}
	return out.ClientID
}

func exchange(t *testing.T, c *http.Client, base string, form url.Values) tokenResp {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, base+auth.TokenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("token: want 200, got %d (%s)", resp.StatusCode, b)
	}
	var out tokenResp
	decode(t, resp.Body, &out)
	return out
}

func callMCP(t *testing.T, c *http.Client, base, token string) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("/mcp: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// mintClientViaServer registers a client through the public handler so tests get
// a valid signed client_id without reaching into unexported minting.
func mintClientViaServer(t *testing.T, a *auth.Authenticator) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"redirect_uris": []string{testRedirect}})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, auth.RegisterPath, strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	a.RegisterHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register helper: want 201, got %d", rec.Code)
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	decode(t, rec.Body, &out)
	return out.ClientID
}

func authorizeReq(t *testing.T, clientID string) *http.Request {
	t.Helper()
	u := auth.AuthorizePath + "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {testRedirect},
		"code_challenge":        {pkceChallenge()},
		"code_challenge_method": {"S256"},
		"state":                 {"s"},
	}.Encode()
	return httptest.NewRequestWithContext(t.Context(), http.MethodGet, u, nil)
}

func decode(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
