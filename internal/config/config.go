// Package config provides configuration loading and validation from environment variables.
// Load() is fail-closed: the server refuses to start unless all required fields are present
// and valid.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all validated server configuration loaded from environment variables.
// Every field is populated (never zero-valued) after a successful Load().
//
// Single-user mode: a single Polar Flow account (POLAR_EMAIL/POLAR_PASSWORD)
// backs every MCP request. The HTTP transport still listens (for stdio-less
// deployment), but it is single-tenant — running multiple instances is the
// supported way to serve multiple accounts.
type Config struct {
	// Transport selects the server transport: "http" (default) or "stdio".
	Transport string

	// BindAddress is the TCP address the HTTP server listens on. Defaults to "127.0.0.1".
	BindAddress string

	// Port is the TCP port the HTTP server listens on. Defaults to "8080".
	Port string

	// PolarEmail is the email used to log into flow.polar.com.
	PolarEmail string

	// PolarPassword is the password used to log into flow.polar.com.
	// Held in memory only — never persisted.
	PolarPassword string

	// CookieJarPath is the path to the chmod-600 JSON file that stores the
	// long-lived session cookies (FLOW_SESSION, remember-me, ...).
	// Defaults to "./polar-cookies.json".
	CookieJarPath string

	// APIKey is the optional fixed pre-shared secret (MCP_API_KEY). When set,
	// every /mcp request must carry it as "Authorization: Bearer <key>". It is
	// the lightweight alternative to OAuth and is mutually exclusive with it.
	APIKey string

	// OAuth holds the optional inbound OAuth 2.1 settings. It is disabled
	// (OAuth.Enabled == false) unless OAUTH_PUBLIC_URL is set, in which case the
	// server becomes its own OAuth 2.1 Authorization Server: it issues tokens
	// (delegating browser login to a forward-auth proxy) and requires a valid
	// Bearer access token on /mcp. See internal/auth for how these are enforced.
	OAuth OAuthConfig
}

// OAuthConfig configures the server as a self-contained OAuth 2.1 Authorization
// Server + Resource Server (the "app-as-AS" model). It is populated from
// environment variables and validated fail-closed by Load(). The zero value
// (Enabled == false) means the HTTP transport keeps its historical behaviour:
// no inbound authentication.
//
// The server implements Dynamic Client Registration (RFC 7591) so MCP clients
// (Claude.ai web/mobile and Claude Code) self-register — no manually-created
// client. Browser login on the /authorize step is delegated to a forward-auth
// proxy (e.g. Authelia in front of Caddy); the proxy authenticates the user and
// passes their identity in a trusted header. An email allowlist decides who may
// consent. Access tokens are short-lived EdDSA JWTs the server signs and
// validates locally — no database.
type OAuthConfig struct {
	// Enabled reports whether inbound OAuth is configured (OAUTH_PUBLIC_URL set).
	Enabled bool

	// PublicURL is this server's public origin and OAuth issuer
	// (OAUTH_PUBLIC_URL), e.g. "https://polar.example.com". It is advertised as
	// the issuer/authorization_server and must match the host Claude connects to.
	PublicURL string

	// Resource is this server's exact public MCP URL, derived as PublicURL+"/mcp".
	// It is the access-token audience and is published in the RFC 9728
	// protected-resource metadata.
	Resource string

	// AllowedEmails is the allowlist of forward-auth identities permitted to
	// complete the consent step (OAUTH_ALLOWED_EMAIL, comma-separated). This is
	// the "lock to me" control; matching is case-insensitive.
	AllowedEmails []string

	// AllowedGroups, when non-empty, additionally requires the forward-auth
	// groups header to intersect this set (OAUTH_ALLOWED_GROUPS).
	AllowedGroups []string

	// AllowedOrigins, when non-empty, enables Origin-header enforcement on /mcp
	// (DNS-rebinding defence). Requests with no Origin are always allowed.
	AllowedOrigins []string

	// EmailHeader is the forward-auth header carrying the authenticated user's
	// email (OAUTH_FORWARD_AUTH_EMAIL_HEADER, default "Remote-Email").
	EmailHeader string

	// GroupsHeader is the forward-auth header carrying the user's groups
	// (OAUTH_FORWARD_AUTH_GROUPS_HEADER, default "Remote-Groups").
	GroupsHeader string

	// TrustedProxies is the set of networks whose forward-auth identity headers
	// are trusted on /authorize (OAUTH_TRUSTED_PROXIES, comma-separated CIDRs or
	// IPs). Required when enabled: an identity header from any other peer is
	// ignored, so this is the control that stops header spoofing.
	TrustedProxies []*net.IPNet

	// ExtraRedirectURIs are additional exact redirect URIs accepted at client
	// registration, beyond the built-in Claude callbacks and loopback
	// (OAUTH_EXTRA_REDIRECT_URIS). Normally empty.
	ExtraRedirectURIs []string

	// SigningKeyPath is the chmod-600 JSON file holding the EdDSA signing key
	// (OAUTH_SIGNING_KEY_PATH, default "./polar-oauth-key.json"). Created on
	// first start; persisting it keeps issued tokens valid across restarts.
	SigningKeyPath string

	// AccessTTL / RefreshTTL are token lifetimes (OAUTH_ACCESS_TTL_MINUTES,
	// default 60; OAUTH_REFRESH_TTL_HOURS, default 720 = 30 days).
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// Load reads configuration from environment variables, validates all required fields, and
// returns a fully populated *Config.
func Load() (*Config, error) {
	cfg := &Config{}

	transport := os.Getenv("TRANSPORT")
	if transport == "" {
		transport = "http"
	}
	switch transport {
	case "http", "stdio":
		cfg.Transport = transport
	default:
		return nil, fmt.Errorf("TRANSPORT must be %q or %q; got %q", "http", "stdio", transport)
	}

	cfg.PolarEmail = os.Getenv("POLAR_EMAIL")
	cfg.PolarPassword = os.Getenv("POLAR_PASSWORD")

	// Credentials are not strictly required at startup IF a cookie jar already
	// exists with a valid session, but to keep failure modes obvious we still
	// require them in env — they are the fallback when remember-me expires.
	if cfg.PolarEmail == "" {
		return nil, errors.New(
			"POLAR_EMAIL must be set to the email of the Polar Flow account this server will act as",
		)
	}
	if cfg.PolarPassword == "" {
		return nil, errors.New(
			"POLAR_PASSWORD must be set to the password of the Polar Flow account",
		)
	}

	cfg.CookieJarPath = os.Getenv("COOKIE_JAR_PATH")
	if cfg.CookieJarPath == "" {
		cfg.CookieJarPath = "./polar-cookies.json"
	}

	cfg.BindAddress = os.Getenv("BIND_ADDRESS")
	if cfg.BindAddress == "" {
		cfg.BindAddress = "127.0.0.1"
	}

	cfg.Port = os.Getenv("PORT")
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if err := loadOAuth(&cfg.OAuth); err != nil {
		return nil, err
	}

	if err := loadAPIKey(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// minAPIKeyLength is the shortest accepted MCP_API_KEY. A fixed shared secret
// is the only thing standing between the internet and the MCP surface, so
// anything a human would plausibly type by hand is refused at startup.
const minAPIKeyLength = 24

// loadAPIKey populates and validates the optional static API key. The two
// inbound auth mechanisms are mutually exclusive: OAuth issues per-client
// tokens it can scope and expire, the API key is one long-lived shared secret,
// and running both would silently weaken the stronger one to the weaker one's
// guarantees.
func loadAPIKey(cfg *Config) error {
	key := strings.TrimSpace(os.Getenv("MCP_API_KEY"))
	if key == "" {
		return nil
	}
	if cfg.OAuth.Enabled {
		return errors.New(
			"MCP_API_KEY and OAUTH_PUBLIC_URL are mutually exclusive: pick either the static " +
				"API key or inbound OAuth, not both",
		)
	}
	if len(key) < minAPIKeyLength {
		return fmt.Errorf(
			"MCP_API_KEY must be at least %d characters (it is the only thing protecting /mcp); "+
				"generate one with: openssl rand -base64 32",
			minAPIKeyLength,
		)
	}
	cfg.APIKey = key
	return nil
}

// loadOAuth populates and validates the optional OAuth config. Auth is enabled
// iff OAUTH_PUBLIC_URL is set; when enabled it is fail-closed — the allowlist
// and trusted proxies are mandatory (without them the server would either trust
// nobody or trust spoofable headers).
func loadOAuth(o *OAuthConfig) error {
	o.PublicURL = strings.TrimRight(os.Getenv("OAUTH_PUBLIC_URL"), "/")
	if o.PublicURL == "" {
		o.Enabled = false
		return nil
	}
	o.Enabled = true

	if err := requireHTTPSURL("OAUTH_PUBLIC_URL", o.PublicURL); err != nil {
		return err
	}
	o.Resource = o.PublicURL + "/mcp"

	o.AllowedEmails = splitCSV(os.Getenv("OAUTH_ALLOWED_EMAIL"))
	if len(o.AllowedEmails) == 0 {
		return errors.New(
			"OAUTH_ALLOWED_EMAIL must list at least one email when OAUTH_PUBLIC_URL is set " +
				"(it is the allowlist of who may connect)",
		)
	}

	proxies, err := parseCIDRs(os.Getenv("OAUTH_TRUSTED_PROXIES"))
	if err != nil {
		return err
	}
	if len(proxies) == 0 {
		return errors.New(
			"OAUTH_TRUSTED_PROXIES must list the proxy network(s) (CIDRs or IPs) whose forward-auth " +
				"identity headers are trusted when OAUTH_PUBLIC_URL is set; without it, identity headers " +
				"would be spoofable",
		)
	}
	o.TrustedProxies = proxies

	o.AllowedGroups = splitCSV(os.Getenv("OAUTH_ALLOWED_GROUPS"))
	o.AllowedOrigins = splitCSV(os.Getenv("OAUTH_ALLOWED_ORIGINS"))
	o.ExtraRedirectURIs = splitCSV(os.Getenv("OAUTH_EXTRA_REDIRECT_URIS"))

	o.EmailHeader = os.Getenv("OAUTH_FORWARD_AUTH_EMAIL_HEADER")
	if o.EmailHeader == "" {
		o.EmailHeader = "Remote-Email"
	}
	o.GroupsHeader = os.Getenv("OAUTH_FORWARD_AUTH_GROUPS_HEADER")
	if o.GroupsHeader == "" {
		o.GroupsHeader = "Remote-Groups"
	}

	o.SigningKeyPath = os.Getenv("OAUTH_SIGNING_KEY_PATH")
	if o.SigningKeyPath == "" {
		o.SigningKeyPath = "./polar-oauth-key.json"
	}

	o.AccessTTL, err = durationEnv("OAUTH_ACCESS_TTL_MINUTES", 60, time.Minute)
	if err != nil {
		return err
	}
	o.RefreshTTL, err = durationEnv("OAUTH_REFRESH_TTL_HOURS", 720, time.Hour)
	if err != nil {
		return err
	}

	return nil
}

// requireHTTPSURL validates that raw is a syntactically valid absolute URL.
// HTTPS is required except for loopback hosts (to allow local testing over
// http://127.0.0.1).
func requireHTTPSURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%s must be an absolute http(s) URL; got %q", name, raw)
	}
	if u.Scheme != "https" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("%s must use https (got %q); http is only allowed for loopback hosts", name, raw)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// parseCIDRs parses a comma-separated list of CIDRs or bare IPs into networks.
// A bare IP becomes a /32 (or /128) host network. Returns nil for empty input.
func parseCIDRs(raw string) ([]*net.IPNet, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]*net.IPNet, 0, len(parts))
	for _, p := range parts {
		if _, n, err := net.ParseCIDR(p); err == nil {
			out = append(out, n)
			continue
		}
		ip := net.ParseIP(p)
		if ip == nil {
			return nil, fmt.Errorf("OAUTH_TRUSTED_PROXIES entry %q is not a valid CIDR or IP", p)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return out, nil
}

// durationEnv reads an integer env var and multiplies it by unit. An unset or
// empty value yields def*unit; a non-integer value is an error.
func durationEnv(name string, def int, unit time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return time.Duration(def) * unit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer; got %q", name, raw)
	}
	return time.Duration(n) * unit, nil
}

// splitCSV parses a comma-separated env value into a trimmed, empties-removed
// slice. Returns nil for an empty/unset value.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// LogStartupBanner emits a structured slog.Info banner. Credentials are NOT
// logged — only the email (for operator identification).
func LogStartupBanner(cfg *Config) {
	slog.Info("polar-flow-mcp starting",
		"transport", cfg.Transport,
		"bind_address", cfg.BindAddress,
		"port", cfg.Port,
		"polar_account", cfg.PolarEmail,
		"cookie_jar", cfg.CookieJarPath,
	)
	if cfg.OAuth.Enabled {
		slog.Info("inbound OAuth enabled (app-as-Authorization-Server + Dynamic Client Registration)",
			"issuer", cfg.OAuth.PublicURL,
			"resource", cfg.OAuth.Resource,
			"allowed_emails", len(cfg.OAuth.AllowedEmails),
			"allowed_groups", len(cfg.OAuth.AllowedGroups) > 0,
			"trusted_proxies", len(cfg.OAuth.TrustedProxies),
			"origin_allowlist", len(cfg.OAuth.AllowedOrigins) > 0,
			"signing_key", cfg.OAuth.SigningKeyPath,
		)
	} else if cfg.APIKey != "" {
		slog.Info("inbound API-key auth enabled — /mcp requires Authorization: Bearer <MCP_API_KEY>")
	} else if cfg.Transport == "http" {
		slog.Info("inbound auth disabled — /mcp is unauthenticated; bind to localhost or front with a trusted proxy")
	}
}
