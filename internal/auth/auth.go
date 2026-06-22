// Package auth turns the HTTP transport into a self-contained OAuth 2.1
// Authorization Server + Resource Server (the "app-as-AS" model).
//
// The server implements Dynamic Client Registration (RFC 7591), so MCP clients
// — Claude.ai web/mobile and Claude Code — self-register by pasting the URL; no
// client is created by hand. Browser login on the /authorize step is delegated
// to a forward-auth proxy (e.g. Authelia in front of Caddy): the proxy
// authenticates the human and passes their identity in a trusted header, and an
// email allowlist decides who may consent. The server signs short-lived EdDSA
// JWT access tokens and validates them locally on /mcp — no database, no
// introspection round-trip.
//
// Nothing here knows about Polar identity: the OAuth flow authenticates the
// human connecting, while the single Polar account stays fixed in env.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config configures the Authorization + Resource Server. It mirrors
// config.OAuthConfig but is kept independent so the auth package does not import
// config (and vice versa).
type Config struct {
	// Issuer is the public origin and OAuth issuer, e.g.
	// "https://polar.example.com". Endpoint URLs are derived from it.
	Issuer string
	// Resource is the exact public /mcp URL; the access-token audience.
	Resource string

	// AllowedEmails is the case-insensitive allowlist of forward-auth identities
	// permitted to consent. AllowedGroups, when set, is additionally required.
	AllowedEmails []string
	AllowedGroups []string
	// AllowedOrigins enables the Origin allowlist on /mcp when non-empty.
	AllowedOrigins []string

	// EmailHeader / GroupsHeader are the forward-auth headers read on /authorize.
	EmailHeader  string
	GroupsHeader string
	// TrustedProxies are the networks whose forward-auth headers are trusted.
	TrustedProxies []*net.IPNet
	// ExtraRedirectURIs are accepted at registration beyond the built-in set.
	ExtraRedirectURIs []string

	// SigningKeyPath is the chmod-600 JSON file holding the EdDSA key.
	SigningKeyPath string
	// AccessTTL / RefreshTTL are issued-token lifetimes.
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Authenticator validates access tokens, serves discovery metadata, and runs
// the authorization-server endpoints (register / authorize / token).
type Authenticator struct {
	cfg Config
	log *slog.Logger

	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	kid  string
}

// New builds an Authenticator, loading or creating the EdDSA signing key at
// cfg.SigningKeyPath (chmod 600). Persisting the key keeps issued tokens valid
// across restarts.
func New(cfg Config) (*Authenticator, error) {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = time.Hour
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 720 * time.Hour
	}

	priv, err := loadOrCreateKey(cfg.SigningKeyPath)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: %w", err)
	}
	pub, _ := priv.Public().(ed25519.PublicKey)

	return &Authenticator{
		cfg:  cfg,
		log:  log,
		priv: priv,
		pub:  pub,
		kid:  keyID(pub),
	}, nil
}

// authorizeURL / tokenURL / registerURL derive the endpoint URLs from the issuer.
func (a *Authenticator) authorizeURL() string { return a.cfg.Issuer + AuthorizePath }
func (a *Authenticator) tokenURL() string     { return a.cfg.Issuer + TokenPath }
func (a *Authenticator) registerURL() string  { return a.cfg.Issuer + RegisterPath }

// allowedEmail reports whether email is on the allowlist (case-insensitive).
func (a *Authenticator) allowedEmail(email string) bool {
	if email == "" {
		return false
	}
	for _, e := range a.cfg.AllowedEmails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	return false
}

// allowedGroups reports whether the caller's groups satisfy the group gate.
// With no AllowedGroups configured the gate is open.
func (a *Authenticator) allowedGroups(groups []string) bool {
	if len(a.cfg.AllowedGroups) == 0 {
		return true
	}
	return intersects(a.cfg.AllowedGroups, groups)
}

// newJTI returns a 128-bit random token identifier.
func newJTI() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func contains(set []string, v string) bool {
	if v == "" {
		return false
	}
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func intersects(set, vals []string) bool {
	for _, v := range vals {
		if contains(set, v) {
			return true
		}
	}
	return false
}

// writeJSON writes v as JSON with the given status and no-store caching.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// oauthError writes an RFC 6749 token/registration error response.
func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// redirectWithError appends an OAuth error to redirectURI and returns the
// location. Used only after redirectURI has been validated against the client.
func redirectWithError(redirectURI, state, code, desc string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	q.Set("error", code)
	if desc != "" {
		q.Set("error_description", desc)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
