// Package config provides configuration loading and validation from environment variables.
// Load() is fail-closed: the server refuses to start unless all required fields are present
// and valid.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
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

	// OAuth holds the optional inbound OAuth 2.1 Resource Server settings.
	// It is disabled (OAuth.Enabled == false) unless OIDC_ISSUER is set, in
	// which case the HTTP transport requires a valid Bearer access token on
	// /mcp. See internal/auth for how these are enforced.
	OAuth OAuthConfig
}

// OAuthConfig configures the server as a provider-agnostic OAuth 2.1 Resource
// Server. It is populated from environment variables and validated fail-closed
// by Load(). The zero value (Enabled == false) means the HTTP transport keeps
// its historical behaviour: no inbound authentication.
//
// Tokens are validated by RFC 7662 introspection against the issuer, so the
// server works with opaque access tokens (Authelia's default) and any OIDC
// provider — point Issuer at Authelia today, Keycloak/Cloudflare/Auth0 later.
type OAuthConfig struct {
	// Enabled reports whether inbound OAuth is configured (OIDC_ISSUER set).
	Enabled bool

	// Issuer is the OIDC issuer base URL (OIDC_ISSUER), e.g.
	// "https://auth.example.com". Its /.well-known/openid-configuration is
	// fetched to discover the introspection endpoint, and it is advertised as
	// the authorization server in the protected-resource metadata.
	Issuer string

	// Resource is this server's exact public MCP URL (MCP_RESOURCE), e.g.
	// "https://polar.example.com/mcp". It MUST byte-match the URL Claude calls
	// and is published verbatim in the RFC 9728 protected-resource metadata.
	Resource string

	// IntrospectionClientID / IntrospectionClientSecret authenticate this
	// server (as an OAuth client) to the issuer's introspection endpoint.
	IntrospectionClientID     string
	IntrospectionClientSecret string

	// The following pins are each optional and enforced only when non-empty.
	// They are ANDed: every configured pin must match or the request is 403.

	// AllowedClientIDs restricts accepted tokens to these client_id values
	// (the connector client Claude was issued). RFC 8707 confused-deputy defence.
	AllowedClientIDs []string
	// AllowedAudiences restricts accepted tokens to these aud values.
	AllowedAudiences []string
	// AllowedSubjects restricts accepted tokens to these sub values.
	AllowedSubjects []string
	// AllowedGroups restricts accepted tokens to these group memberships.
	AllowedGroups []string
	// AllowedOrigins, when non-empty, enables Origin-header enforcement
	// (DNS-rebinding defence). Requests with no Origin are always allowed.
	AllowedOrigins []string
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

	return cfg, nil
}

// loadOAuth populates and validates the optional OAuth Resource Server config.
// Auth is enabled iff OIDC_ISSUER is set; when enabled it is fail-closed —
// MCP_RESOURCE and introspection credentials are mandatory.
func loadOAuth(o *OAuthConfig) error {
	o.Issuer = strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/")
	if o.Issuer == "" {
		o.Enabled = false
		return nil
	}
	o.Enabled = true

	o.Resource = os.Getenv("MCP_RESOURCE")
	o.IntrospectionClientID = os.Getenv("OIDC_INTROSPECTION_CLIENT_ID")
	o.IntrospectionClientSecret = os.Getenv("OIDC_INTROSPECTION_CLIENT_SECRET")

	if err := requireHTTPSURL("OIDC_ISSUER", o.Issuer); err != nil {
		return err
	}
	if o.Resource == "" {
		return errors.New("MCP_RESOURCE must be set (the exact public /mcp URL) when OIDC_ISSUER is configured")
	}
	if err := requireHTTPSURL("MCP_RESOURCE", o.Resource); err != nil {
		return err
	}
	if o.IntrospectionClientID == "" || o.IntrospectionClientSecret == "" {
		return errors.New(
			"OIDC_INTROSPECTION_CLIENT_ID and OIDC_INTROSPECTION_CLIENT_SECRET must be set " +
				"when OIDC_ISSUER is configured (the server validates tokens via RFC 7662 introspection)",
		)
	}

	o.AllowedClientIDs = splitCSV(os.Getenv("AUTH_ALLOWED_CLIENT_IDS"))
	o.AllowedAudiences = splitCSV(os.Getenv("AUTH_ALLOWED_AUDIENCES"))
	o.AllowedSubjects = splitCSV(os.Getenv("AUTH_ALLOWED_SUBJECTS"))
	o.AllowedGroups = splitCSV(os.Getenv("AUTH_ALLOWED_GROUPS"))
	o.AllowedOrigins = splitCSV(os.Getenv("AUTH_ALLOWED_ORIGINS"))

	return nil
}

// requireHTTPSURL validates that raw is a syntactically valid absolute URL.
// HTTPS is required except for loopback hosts (to allow local Authelia and
// tests over http://127.0.0.1).
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
		slog.Info("inbound OAuth enabled (Resource Server, RFC 7662 introspection)",
			"issuer", cfg.OAuth.Issuer,
			"resource", cfg.OAuth.Resource,
			"introspection_client", cfg.OAuth.IntrospectionClientID,
			"pin_client_ids", len(cfg.OAuth.AllowedClientIDs) > 0,
			"pin_audiences", len(cfg.OAuth.AllowedAudiences) > 0,
			"pin_subjects", len(cfg.OAuth.AllowedSubjects) > 0,
			"pin_groups", len(cfg.OAuth.AllowedGroups) > 0,
			"origin_allowlist", len(cfg.OAuth.AllowedOrigins) > 0,
		)
	} else if cfg.Transport == "http" {
		slog.Info("inbound OAuth disabled — /mcp is unauthenticated; bind to localhost or front with a trusted proxy")
	}
}
