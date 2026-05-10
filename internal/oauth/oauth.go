// Package oauth provides HTTP handlers for the Polar OAuth2 authorization flow (per D-03..D-06).
package oauth

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/config"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/polar"
	"github.com/lm/polar-flow-mcp/internal/store"
)

// authorizeURL is the Polar authorization page URL. Overridable in tests via export_test.go.
//
//nolint:gochecknoglobals
var authorizeURL = "https://flow.polar.com/oauth2/authorization"

// stateTTL is the lifetime of an OAuth CSRF state row in pending_auth.
const stateTTL = 10 * time.Minute

// keyVersion is the encryption key version persisted in polar_tokens.key_version (per D-05).
const keyVersion = 1

// Handlers holds the dependencies for the OAuth flow HTTP handlers.
type Handlers struct {
	cfg    *config.Config
	st     *store.Store
	cipher *crypto.Cipher
}

// NewHandlers creates a Handlers instance with all required dependencies.
func NewHandlers(cfg *config.Config, st *store.Store, cipher *crypto.Cipher) *Handlers {
	return &Handlers{cfg: cfg, st: st, cipher: cipher}
}

// Login initiates the Polar OAuth2 authorization flow (per OAUTH-01, D-03).
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		slog.Warn("oauth login: missing identity in context — auth middleware not applied")
		htmlError(w, http.StatusInternalServerError, "missing identity")
		return
	}

	stateBytes := make([]byte, 32)
	if _, err := io.ReadFull(cryptorand.Reader, stateBytes); err != nil {
		slog.Error("oauth login: failed to generate state", "error", err)
		htmlError(w, http.StatusInternalServerError, "failed to generate authorization state")
		return
	}
	state := hex.EncodeToString(stateBytes)
	expiresAt := time.Now().UTC().Add(stateTTL)

	if err := h.st.CreateOAuthState(r.Context(), state, identity, expiresAt); err != nil {
		slog.Error("oauth login: failed to store state", "error", err)
		htmlError(w, http.StatusInternalServerError, "failed to store authorization state")
		return
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {h.cfg.PolarClientID},
		"redirect_uri":  {h.cfg.PolarRedirectURL},
		"scope":         {"accesslink.read_all"},
		"state":         {state},
	}
	http.Redirect(w, r, authorizeURL+"?"+params.Encode(), http.StatusFound)
}

// Callback handles the Polar OAuth2 callback (per OAUTH-02..OAUTH-05, D-04, D-05, D-06).
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		htmlError(w, http.StatusBadRequest, "missing state or code parameter")
		return
	}

	// Atomically consume the state — single-use (D-04).
	stateIdentity, err := h.st.ConsumeOAuthState(r.Context(), state)
	if errors.Is(err, store.ErrNotFound) {
		htmlError(w, http.StatusBadRequest, "invalid or expired authorization state")
		return
	}
	if errors.Is(err, store.ErrExpired) {
		htmlError(w, http.StatusBadRequest, "authorization session expired — please restart from /oauth/login")
		return
	}
	if err != nil {
		slog.Error("oauth callback: state validation error", "error", err)
		htmlError(w, http.StatusInternalServerError, "state validation error")
		return
	}

	// Identity bound at /oauth/login MUST match current request identity (CSRF check).
	currentIdentity, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		slog.Warn("oauth callback: missing identity in context")
		htmlError(w, http.StatusInternalServerError, "missing identity")
		return
	}
	if stateIdentity != currentIdentity {
		slog.Warn("oauth callback identity mismatch — possible CSRF",
			"state_identity", stateIdentity,
			"request_identity", currentIdentity,
		)
		htmlError(w, http.StatusBadRequest, "identity mismatch")
		return
	}

	// Exchange code for token (OAUTH-03).
	tr, err := polar.ExchangeCode(r.Context(), h.cfg.PolarClientID, h.cfg.PolarClientSecret, code, h.cfg.PolarRedirectURL)
	if err != nil {
		slog.Error("oauth callback: token exchange failed", "error", err, "identity", currentIdentity)
		htmlError(w, http.StatusBadGateway, "Polar token exchange failed")
		return
	}

	// Register user with Polar (OAUTH-04). 409 = idempotent; use x_user_id from token exchange in all cases.
	polarUserID := strconv.FormatInt(tr.XUserID, 10) // schema is TEXT (also reused for the member-id field below).
	if _, err := polar.RegisterUser(r.Context(), tr.AccessToken, polarUserID); err != nil {
		slog.Error("oauth callback: user registration failed", "error", err, "identity", currentIdentity)
		htmlError(w, http.StatusBadGateway, "Polar user registration failed")
		return
	}

	// Persist user row first (FK requirement).
	if err := h.st.UpsertUser(r.Context(), currentIdentity, polarUserID); err != nil {
		slog.Error("oauth callback: upsert user failed", "error", err, "identity", currentIdentity)
		htmlError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	// Encrypt token (AES-256-GCM, nonce||ciphertext) and upsert (OAUTH-05).
	blob, err := h.cipher.Encrypt([]byte(tr.AccessToken))
	if err != nil {
		slog.Error("oauth callback: encrypt token failed", "error", err, "identity", currentIdentity)
		htmlError(w, http.StatusInternalServerError, "failed to encrypt token")
		return
	}
	if err := h.st.UpsertToken(r.Context(), currentIdentity, blob, keyVersion); err != nil {
		slog.Error("oauth callback: upsert token failed", "error", err, "identity", currentIdentity)
		htmlError(w, http.StatusInternalServerError, "failed to save token")
		return
	}

	htmlSuccess(w)
}

// htmlSuccess writes the Polar-linked confirmation page (per D-06).
func htmlSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Polar Linked</title>
<style>body{font-family:sans-serif;text-align:center;padding:4rem}.check{font-size:4rem;color:#2ecc71}</style></head>
<body><div class="check">&#10004;</div>
<h1>Polar account linked</h1>
<p>You may close this tab and return to Claude.</p>
</body></html>`)
}

// htmlError writes a self-contained error page (per D-06). msg is HTML-escaped before embedding.
func htmlError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Error</title>
<style>body{font-family:sans-serif;text-align:center;padding:4rem}.x{font-size:4rem;color:#e74c3c}</style></head>
<body><div class="x">&#10008;</div>
<h1>Link failed</h1><p>%s</p>
</body></html>`, html.EscapeString(msg))
}
