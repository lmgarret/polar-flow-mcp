// Package config_test tests fail-closed validation in config.Load().
package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/config"
)

// clearConfigEnv clears all environment variables read by config.Load().
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"TRANSPORT",
		"BIND_ADDRESS",
		"PORT",
		"POLAR_EMAIL",
		"POLAR_PASSWORD",
		"COOKIE_JAR_PATH",
		"OAUTH_PUBLIC_URL",
		"OAUTH_ALLOWED_EMAIL",
		"OAUTH_ALLOWED_GROUPS",
		"OAUTH_ALLOWED_ORIGINS",
		"OAUTH_TRUSTED_PROXIES",
		"OAUTH_FORWARD_AUTH_EMAIL_HEADER",
		"OAUTH_FORWARD_AUTH_GROUPS_HEADER",
		"OAUTH_EXTRA_REDIRECT_URIS",
		"OAUTH_SIGNING_KEY_PATH",
		"OAUTH_ACCESS_TTL_MINUTES",
		"OAUTH_REFRESH_TTL_HOURS",
	} {
		t.Setenv(key, "")
	}
}

// withCreds sets the always-required Polar credentials.
func withCreds(t *testing.T) {
	t.Helper()
	t.Setenv("POLAR_EMAIL", "me@example.com")
	t.Setenv("POLAR_PASSWORD", "x")
}

func TestLoad_OAuthDisabledByDefault(t *testing.T) {
	clearConfigEnv(t)
	withCreds(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OAuth.Enabled {
		t.Error("OAuth should be disabled when OAUTH_PUBLIC_URL is unset")
	}
}

func TestLoad_OAuthValidation(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "missing allowed email",
			env: map[string]string{
				"OAUTH_PUBLIC_URL":      "https://polar.example.com",
				"OAUTH_TRUSTED_PROXIES": "10.0.0.0/8",
			},
			want: "OAUTH_ALLOWED_EMAIL",
		},
		{
			name: "missing trusted proxies",
			env: map[string]string{
				"OAUTH_PUBLIC_URL":    "https://polar.example.com",
				"OAUTH_ALLOWED_EMAIL": "me@example.com",
			},
			want: "OAUTH_TRUSTED_PROXIES",
		},
		{
			name: "non-https public url",
			env: map[string]string{
				"OAUTH_PUBLIC_URL":      "http://polar.example.com",
				"OAUTH_ALLOWED_EMAIL":   "me@example.com",
				"OAUTH_TRUSTED_PROXIES": "10.0.0.0/8",
			},
			want: "https",
		},
		{
			name: "bad cidr",
			env: map[string]string{
				"OAUTH_PUBLIC_URL":      "https://polar.example.com",
				"OAUTH_ALLOWED_EMAIL":   "me@example.com",
				"OAUTH_TRUSTED_PROXIES": "not-an-ip",
			},
			want: "OAUTH_TRUSTED_PROXIES",
		},
		{
			name: "bad ttl",
			env: map[string]string{
				"OAUTH_PUBLIC_URL":         "https://polar.example.com",
				"OAUTH_ALLOWED_EMAIL":      "me@example.com",
				"OAUTH_TRUSTED_PROXIES":    "10.0.0.0/8",
				"OAUTH_ACCESS_TTL_MINUTES": "zero",
			},
			want: "OAUTH_ACCESS_TTL_MINUTES",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearConfigEnv(t)
			withCreds(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should mention %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestLoad_OAuthEnabledFull(t *testing.T) {
	clearConfigEnv(t)
	withCreds(t)
	t.Setenv("OAUTH_PUBLIC_URL", "https://polar.example.com/") // trailing slash trimmed
	t.Setenv("OAUTH_ALLOWED_EMAIL", "me@example.com, ")
	t.Setenv("OAUTH_TRUSTED_PROXIES", "10.0.0.0/8, 172.18.0.5")
	t.Setenv("OAUTH_ALLOWED_GROUPS", "polar")
	t.Setenv("OAUTH_ALLOWED_ORIGINS", "https://claude.ai")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o := cfg.OAuth
	if !o.Enabled {
		t.Fatal("OAuth should be enabled")
	}
	if o.PublicURL != "https://polar.example.com" {
		t.Errorf("PublicURL trailing slash not trimmed: %q", o.PublicURL)
	}
	if o.Resource != "https://polar.example.com/mcp" {
		t.Errorf("Resource: want .../mcp, got %q", o.Resource)
	}
	if len(o.AllowedEmails) != 1 || o.AllowedEmails[0] != "me@example.com" {
		t.Errorf("AllowedEmails: got %v", o.AllowedEmails)
	}
	if len(o.TrustedProxies) != 2 {
		t.Errorf("TrustedProxies: want 2, got %v", o.TrustedProxies)
	}
	if o.EmailHeader != "Remote-Email" || o.GroupsHeader != "Remote-Groups" {
		t.Errorf("default forward-auth headers wrong: %q / %q", o.EmailHeader, o.GroupsHeader)
	}
	if o.AccessTTL != 60*time.Minute {
		t.Errorf("default AccessTTL: want 60m, got %v", o.AccessTTL)
	}
	if o.RefreshTTL != 720*time.Hour {
		t.Errorf("default RefreshTTL: want 720h, got %v", o.RefreshTTL)
	}
	if o.SigningKeyPath != "./polar-oauth-key.json" {
		t.Errorf("default SigningKeyPath: got %q", o.SigningKeyPath)
	}
}

func TestLoad_OAuthAllowsLoopbackHTTP(t *testing.T) {
	clearConfigEnv(t)
	withCreds(t)
	t.Setenv("OAUTH_PUBLIC_URL", "http://127.0.0.1:8080")
	t.Setenv("OAUTH_ALLOWED_EMAIL", "me@example.com")
	t.Setenv("OAUTH_TRUSTED_PROXIES", "127.0.0.1")
	if _, err := config.Load(); err != nil {
		t.Fatalf("loopback http should be allowed: %v", err)
	}
}

func TestLoad_MissingEmail(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "stdio")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing POLAR_EMAIL")
	}
	if !strings.Contains(err.Error(), "POLAR_EMAIL") {
		t.Errorf("error should mention POLAR_EMAIL, got: %v", err)
	}
}

func TestLoad_MissingPassword(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "stdio")
	t.Setenv("POLAR_EMAIL", "me@example.com")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing POLAR_PASSWORD")
	}
	if !strings.Contains(err.Error(), "POLAR_PASSWORD") {
		t.Errorf("error should mention POLAR_PASSWORD, got: %v", err)
	}
}

func TestLoad_BadTransport(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "websocket")
	t.Setenv("POLAR_EMAIL", "me@example.com")
	t.Setenv("POLAR_PASSWORD", "x")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for unknown TRANSPORT")
	}
	if !strings.Contains(err.Error(), "TRANSPORT") {
		t.Errorf("error should mention TRANSPORT, got: %v", err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("POLAR_EMAIL", "me@example.com")
	t.Setenv("POLAR_PASSWORD", "x")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport != "http" {
		t.Errorf("default Transport: want http, got %q", cfg.Transport)
	}
	if cfg.CookieJarPath != "./polar-cookies.json" {
		t.Errorf("default CookieJarPath: want ./polar-cookies.json, got %q", cfg.CookieJarPath)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("default BindAddress: want 127.0.0.1, got %q", cfg.BindAddress)
	}
	if cfg.Port != "8080" {
		t.Errorf("default Port: want 8080, got %q", cfg.Port)
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "stdio")
	t.Setenv("POLAR_EMAIL", "me@example.com")
	t.Setenv("POLAR_PASSWORD", "x")
	t.Setenv("COOKIE_JAR_PATH", "/var/lib/polar/jar.json")
	t.Setenv("BIND_ADDRESS", "0.0.0.0")
	t.Setenv("PORT", "9000")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport != "stdio" {
		t.Errorf("Transport: want stdio, got %q", cfg.Transport)
	}
	if cfg.CookieJarPath != "/var/lib/polar/jar.json" {
		t.Errorf("CookieJarPath: got %q", cfg.CookieJarPath)
	}
	if cfg.BindAddress != "0.0.0.0" {
		t.Errorf("BindAddress: got %q", cfg.BindAddress)
	}
	if cfg.Port != "9000" {
		t.Errorf("Port: got %q", cfg.Port)
	}
}
