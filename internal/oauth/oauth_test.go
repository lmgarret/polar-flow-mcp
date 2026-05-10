package oauth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/config"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/oauth"
	"github.com/lm/polar-flow-mcp/internal/polar"
	"github.com/lm/polar-flow-mcp/internal/store"
)

func testConfig() *config.Config {
	return &config.Config{
		PolarClientID:     "cid",
		PolarClientSecret: "sec",
		PolarRedirectURL:  "https://example.com/cb",
	}
}

func testCipher() *crypto.Cipher {
	return crypto.NewCipher(crypto.NewBytesKeyProvider(make([]byte, 32)))
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	s, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func injectIdentity(r *http.Request, identity string) *http.Request {
	ctx := context.WithValue(r.Context(), auth.UserIDKey, identity)
	return r.WithContext(ctx)
}

func TestLoginHandler_RedirectsAndStoresState(t *testing.T) {
	s := testStore(t)
	h := oauth.NewHandlers(testConfig(), s, testCipher())

	// Override authorize URL to a test server to capture where we'd redirect
	// (but we just test the redirect Location directly without hitting it)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/login", nil)
	req = injectIdentity(req, "alice")
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}

	loc := rr.Header().Get("Location")
	if loc == "" {
		t.Fatal("Location header is empty")
	}

	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location %q: %v", loc, err)
	}
	if u.Host != "flow.polar.com" {
		t.Errorf("host = %q, want flow.polar.com", u.Host)
	}
	if u.Path != "/oauth2/authorization" {
		t.Errorf("path = %q, want /oauth2/authorization", u.Path)
	}
	q := u.Query()
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want code", q.Get("response_type"))
	}
	if q.Get("client_id") != "cid" {
		t.Errorf("client_id = %q, want cid", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "https://example.com/cb" {
		t.Errorf("redirect_uri = %q, want https://example.com/cb", q.Get("redirect_uri"))
	}
	if q.Get("scope") != "accesslink.read_all" {
		t.Errorf("scope = %q, want accesslink.read_all", q.Get("scope"))
	}
	state := q.Get("state")
	matched, _ := regexp.MatchString(`^[0-9a-f]{64}$`, state)
	if !matched {
		t.Errorf("state = %q, want 64-char hex string", state)
	}

	// Check pending_auth has one row for alice
	var identity string
	var expiresAt time.Time
	row := s.ReadDB().QueryRowContext(context.Background(),
		`SELECT identity, expires_at FROM pending_auth WHERE state = ?`, state)
	if err := row.Scan(&identity, &expiresAt); err != nil {
		t.Fatalf("no pending_auth row for state %q: %v", state, err)
	}
	if identity != "alice" {
		t.Errorf("identity = %q, want alice", identity)
	}
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		t.Errorf("expiresAt %v is not in the future", expiresAt)
	}
	if !expiresAt.Before(now.Add(11 * time.Minute)) {
		t.Errorf("expiresAt %v is more than 11 minutes from now", expiresAt)
	}
}

func TestLoginHandler_NoIdentity_Returns500(t *testing.T) {
	s := testStore(t)
	h := oauth.NewHandlers(testConfig(), s, testCipher())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/login", nil)
	// No identity injected
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code >= http.StatusOK && rr.Code < http.StatusMultipleChoices {
		t.Errorf("status = %d, want non-2xx", rr.Code)
	}

	// No row should be inserted
	var count int
	_ = s.ReadDB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pending_auth`).Scan(&count)
	if count != 0 {
		t.Errorf("pending_auth has %d rows, want 0", count)
	}
}

func TestCallbackHandler_InvalidState_Returns400(t *testing.T) {
	s := testStore(t)
	h := oauth.NewHandlers(testConfig(), s, testCipher())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/callback?state=missing&code=anything", nil)
	req = injectIdentity(req, "alice")
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if !contains(rr.Body.String(), "invalid or expired") {
		t.Errorf("body %q should contain 'invalid or expired'", rr.Body.String())
	}
}

func TestCallbackHandler_ExpiredState_Returns400(t *testing.T) {
	s := testStore(t)
	h := oauth.NewHandlers(testConfig(), s, testCipher())

	// Pre-insert expired state
	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	if err := s.CreateOAuthState(context.Background(), "expiredstate", "alice", expiredAt); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/callback?state=expiredstate&code=anything", nil)
	req = injectIdentity(req, "alice")
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if !contains(rr.Body.String(), "expired") {
		t.Errorf("body %q should contain 'expired'", rr.Body.String())
	}
}

func TestCallbackHandler_IdentityMismatch_Returns400(t *testing.T) {
	s := testStore(t)
	h := oauth.NewHandlers(testConfig(), s, testCipher())

	// State bound to alice
	if err := s.CreateOAuthState(context.Background(), "mismatchstate", "alice", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	// Request comes in as bob
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/callback?state=mismatchstate&code=anything", nil)
	req = injectIdentity(req, "bob")
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if !contains(rr.Body.String(), "identity mismatch") {
		t.Errorf("body %q should contain 'identity mismatch'", rr.Body.String())
	}
}

func TestCallbackHandler_Success(t *testing.T) {
	s := testStore(t)

	// Mock token endpoint
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"token_type":   "bearer",
			"x_user_id":    777,
		})
	}))
	defer tokenSrv.Close()
	restoreToken := polar.SetTokenEndpoint(tokenSrv.URL)
	t.Cleanup(restoreToken)

	// Mock register endpoint — asserts member-id is the x_user_id string, not the access token (CR-02).
	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(bodyBytes, &payload)
		if payload["member-id"] != "777" {
			t.Errorf("member-id = %q, want \"777\" (CR-02 regression)", payload["member-id"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"polar-user-id": 777})
	}))
	defer regSrv.Close()
	restoreReg := polar.SetRegisterEndpoint(regSrv.URL)
	t.Cleanup(restoreReg)

	h := oauth.NewHandlers(testConfig(), s, testCipher())

	// Pre-insert state for alice
	if err := s.CreateOAuthState(context.Background(), "successstate", "alice", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/callback?state=successstate&code=authcode", nil)
	req = injectIdentity(req, "alice")
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !contains(rr.Body.String(), "Polar account linked") {
		t.Errorf("body %q should contain 'Polar account linked'", rr.Body.String())
	}

	// Assert users row
	var polarUserID string
	err := s.ReadDB().QueryRowContext(context.Background(),
		`SELECT polar_user_id FROM users WHERE identity = ?`, "alice").Scan(&polarUserID)
	if err != nil {
		t.Fatalf("users query: %v", err)
	}
	if polarUserID != strconv.FormatInt(777, 10) {
		t.Errorf("polar_user_id = %q, want 777", polarUserID)
	}

	// Assert polar_tokens row
	var blobLen int
	err = s.ReadDB().QueryRowContext(context.Background(),
		`SELECT length(encrypted_token) FROM polar_tokens pt JOIN users u ON pt.user_id = u.id WHERE u.identity = ?`,
		"alice").Scan(&blobLen)
	if err != nil {
		t.Fatalf("polar_tokens query: %v", err)
	}
	if blobLen < 12 {
		t.Errorf("encrypted_token length = %d, want >= 12", blobLen)
	}

	// State row should be deleted (replay returns 400)
	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/callback?state=successstate&code=authcode", nil)
	req2 = injectIdentity(req2, "alice")
	rr2 := httptest.NewRecorder()
	h.Callback(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("replay status = %d, want 400", rr2.Code)
	}
}

func TestCallbackHandler_RegistrationConflict_UsesXUserID(t *testing.T) {
	s := testStore(t)

	// Mock token endpoint — returns x_user_id=777
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"token_type":   "bearer",
			"x_user_id":    777,
		})
	}))
	defer tokenSrv.Close()
	restoreToken := polar.SetTokenEndpoint(tokenSrv.URL)
	t.Cleanup(restoreToken)

	// Mock register endpoint — returns 409 (already registered)
	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer regSrv.Close()
	restoreReg := polar.SetRegisterEndpoint(regSrv.URL)
	t.Cleanup(restoreReg)

	h := oauth.NewHandlers(testConfig(), s, testCipher())

	// Pre-insert state for alice
	if err := s.CreateOAuthState(context.Background(), "conflictstate", "alice", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth/callback?state=conflictstate&code=authcode", nil)
	req = injectIdentity(req, "alice")
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// polar_user_id must be 777 (from x_user_id)
	var polarUserID string
	err := s.ReadDB().QueryRowContext(context.Background(),
		`SELECT polar_user_id FROM users WHERE identity = ?`, "alice").Scan(&polarUserID)
	if err != nil {
		t.Fatalf("users query: %v", err)
	}
	if polarUserID != strconv.FormatInt(777, 10) {
		t.Errorf("polar_user_id = %q, want 777", polarUserID)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
