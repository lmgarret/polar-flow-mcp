package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func testAuth(t *testing.T) *Authenticator {
	t.Helper()
	_, n, _ := net.ParseCIDR("192.0.2.0/24")
	a, err := New(Config{
		Issuer:         "http://polar.test",
		Resource:       "http://polar.test/mcp",
		AllowedEmails:  []string{"me@example.com"},
		EmailHeader:    "Remote-Email",
		TrustedProxies: []*net.IPNet{n},
		SigningKeyPath: filepath.Join(t.TempDir(), "key.json"),
		AccessTTL:      time.Hour,
		RefreshTTL:     time.Hour,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestTokenTypeConfusion ensures non-access tokens are rejected on /mcp, even
// though the server signed them.
func TestTokenTypeConfusion(t *testing.T) {
	a := testAuth(t)
	refresh, err := a.mintRefresh("me@example.com", "cid", "openid")
	if err != nil {
		t.Fatal(err)
	}
	code, err := a.mintCode("me@example.com", "cid", "https://claude.ai/api/mcp/auth_callback", "x", "S256", "openid")
	if err != nil {
		t.Fatal(err)
	}
	for name, tok := range map[string]string{"refresh": refresh, "code": code} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			rec := httptest.NewRecorder()
			a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s token must not be accepted on /mcp, got %d", name, rec.Code)
			}
		})
	}
}

// TestAccessWrongAudienceRejected guards the confused-deputy defence.
func TestAccessWrongAudienceRejected(t *testing.T) {
	a := testAuth(t)
	// Mint an access token then point the verifier at a different resource.
	access, _, err := a.mintAccess("me@example.com", "openid", nil)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg.Resource = "http://other.test/mcp"

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token for another audience must be rejected, got %d", rec.Code)
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := "the-quick-brown-fox-jumps-over-the-lazy-dog!!"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !verifyPKCE(challenge, "S256", verifier) {
		t.Error("matching verifier should pass")
	}
	if verifyPKCE(challenge, "S256", "wrong") {
		t.Error("wrong verifier should fail")
	}
	if verifyPKCE(challenge, "plain", verifier) {
		t.Error("plain method must be rejected")
	}
}

func TestRedirectAllowed(t *testing.T) {
	a := testAuth(t)
	a.cfg.ExtraRedirectURIs = []string{"https://my.app/cb"}
	good := []string{
		"https://claude.ai/api/mcp/auth_callback",
		"https://claude.com/api/mcp/auth_callback",
		"http://127.0.0.1:51000/callback",
		"http://localhost/cb",
		"https://my.app/cb",
	}
	for _, u := range good {
		if !a.redirectAllowed(u) {
			t.Errorf("should allow %q", u)
		}
	}
	bad := []string{
		"https://evil.example/cb",
		"http://10.0.0.5/cb",
		"https://claude.ai/other",
		"ftp://127.0.0.1/cb",
	}
	for _, u := range bad {
		if a.redirectAllowed(u) {
			t.Errorf("should reject %q", u)
		}
	}
}

func TestPeerTrusted(t *testing.T) {
	a := testAuth(t) // trusts 192.0.2.0/24
	if !a.peerTrusted("192.0.2.7:1234") {
		t.Error("in-range peer should be trusted")
	}
	if a.peerTrusted("8.8.8.8:1234") {
		t.Error("out-of-range peer should not be trusted")
	}
	if a.peerTrusted("garbage") {
		t.Error("unparseable peer should not be trusted")
	}
}
