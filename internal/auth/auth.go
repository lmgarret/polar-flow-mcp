// Package auth turns the HTTP transport into a provider-agnostic OAuth 2.1
// Resource Server. Incoming Bearer access tokens on /mcp are validated by
// RFC 7662 token introspection against the configured OIDC issuer, so the
// server works with opaque tokens (Authelia's default) and any compliant
// provider.
//
// Nothing here knows about Polar identity: the OAuth flow authenticates the
// human/agent connecting (against the issuer, e.g. Authelia), while the single
// Polar account stays fixed in env. The middleware only answers "is this token
// active, and is it one this server should accept?".
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config configures the Resource Server. It mirrors config.OAuthConfig but is
// kept independent so the auth package does not import config (and vice versa).
type Config struct {
	Issuer                    string
	Resource                  string
	IntrospectionClientID     string
	IntrospectionClientSecret string

	AllowedClientIDs []string
	AllowedAudiences []string
	AllowedSubjects  []string
	AllowedGroups    []string
	AllowedOrigins   []string

	// HTTPClient is used for discovery and introspection. Defaults to a client
	// with a 10s timeout. This talks to the operator's own issuer, not Polar,
	// so it needs no browser-fingerprint transport.
	HTTPClient *http.Client
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Authenticator validates Bearer tokens and serves protected-resource metadata.
type Authenticator struct {
	cfg    Config
	client *http.Client
	log    *slog.Logger

	mu                    sync.Mutex
	introspectionEndpoint string
}

// New builds an Authenticator. It performs no network I/O; discovery happens
// lazily on the first request (and is cached).
func New(cfg Config) *Authenticator {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Authenticator{cfg: cfg, client: client, log: log}
}

// openIDConfiguration is the subset of the OIDC discovery document we need.
type openIDConfiguration struct {
	IntrospectionEndpoint string `json:"introspection_endpoint"`
}

// introspectionResult is the subset of an RFC 7662 introspection response we
// act on. aud and groups may arrive as a string or an array, so they use a
// lenient decoder.
type introspectionResult struct {
	Active   bool          `json:"active"`
	ClientID string        `json:"client_id"`
	Subject  string        `json:"sub"`
	Audience stringOrSlice `json:"aud"`
	Groups   stringOrSlice `json:"groups"`
	Scope    string        `json:"scope"`
	Username string        `json:"username"`
}

// discover resolves and caches the issuer's introspection endpoint.
func (a *Authenticator) discover(ctx context.Context) (string, error) {
	a.mu.Lock()
	cached := a.introspectionEndpoint
	a.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	wellKnown := a.cfg.Issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return "", fmt.Errorf("build discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", wellKnown, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discovery %s returned %d", wellKnown, resp.StatusCode)
	}

	var doc openIDConfiguration
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return "", fmt.Errorf("decode discovery document: %w", err)
	}
	if doc.IntrospectionEndpoint == "" {
		return "", fmt.Errorf("issuer %q advertises no introspection_endpoint", a.cfg.Issuer)
	}

	a.mu.Lock()
	a.introspectionEndpoint = doc.IntrospectionEndpoint
	a.mu.Unlock()
	return doc.IntrospectionEndpoint, nil
}

// introspect validates a token at the issuer. A nil error with active==true
// means the token is currently valid.
func (a *Authenticator) introspect(ctx context.Context, token string) (*introspectionResult, error) {
	endpoint, err := a.discover(ctx)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", "access_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(a.cfg.IntrospectionClientID, a.cfg.IntrospectionClientSecret)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection endpoint returned %d", resp.StatusCode)
	}

	var res introspectionResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode introspection response: %w", err)
	}
	return &res, nil
}

// errPinMismatch is returned by checkPins describing which configured pin failed.
var errPinMismatch = errors.New("token rejected by configured pin")

// checkPins enforces the configured audience/client/subject/group pins. All
// configured pins must pass (logical AND). A nil error means accepted.
func (a *Authenticator) checkPins(res *introspectionResult) error {
	if len(a.cfg.AllowedClientIDs) > 0 && !contains(a.cfg.AllowedClientIDs, res.ClientID) {
		return fmt.Errorf("%w: client_id %q not allowed", errPinMismatch, res.ClientID)
	}
	if len(a.cfg.AllowedAudiences) > 0 && !intersects(a.cfg.AllowedAudiences, res.Audience) {
		return fmt.Errorf("%w: audience %v not allowed", errPinMismatch, []string(res.Audience))
	}
	if len(a.cfg.AllowedSubjects) > 0 && !contains(a.cfg.AllowedSubjects, res.Subject) {
		return fmt.Errorf("%w: subject %q not allowed", errPinMismatch, res.Subject)
	}
	if len(a.cfg.AllowedGroups) > 0 && !intersects(a.cfg.AllowedGroups, res.Groups) {
		return fmt.Errorf("%w: groups %v not allowed", errPinMismatch, []string(res.Groups))
	}
	return nil
}

// stringOrSlice decodes a JSON value that may be either a string or an array of
// strings into a []string. Other types decode to nil rather than erroring.
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err == nil {
		*s = many
		return nil
	}
	*s = nil
	return nil
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

func intersects(set []string, vals []string) bool {
	for _, v := range vals {
		if contains(set, v) {
			return true
		}
	}
	return false
}
