// Package config_test tests fail-closed validation in config.Load().
package config_test

import (
	"strings"
	"testing"

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
		"OIDC_ISSUER",
		"MCP_RESOURCE",
		"OIDC_INTROSPECTION_CLIENT_ID",
		"OIDC_INTROSPECTION_CLIENT_SECRET",
		"AUTH_ALLOWED_CLIENT_IDS",
		"AUTH_ALLOWED_AUDIENCES",
		"AUTH_ALLOWED_SUBJECTS",
		"AUTH_ALLOWED_GROUPS",
		"AUTH_ALLOWED_ORIGINS",
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
		t.Error("OAuth should be disabled when OIDC_ISSUER is unset")
	}
}

func TestLoad_OAuthRequiresResourceAndCreds(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "missing resource",
			env:  map[string]string{"OIDC_ISSUER": "https://auth.example.com"},
			want: "MCP_RESOURCE",
		},
		{
			name: "missing introspection creds",
			env: map[string]string{
				"OIDC_ISSUER":  "https://auth.example.com",
				"MCP_RESOURCE": "https://polar.example.com/mcp",
			},
			want: "OIDC_INTROSPECTION_CLIENT_ID",
		},
		{
			name: "non-https issuer",
			env: map[string]string{
				"OIDC_ISSUER":                      "http://auth.example.com",
				"MCP_RESOURCE":                     "https://polar.example.com/mcp",
				"OIDC_INTROSPECTION_CLIENT_ID":     "rs",
				"OIDC_INTROSPECTION_CLIENT_SECRET": "s",
			},
			want: "https",
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
	t.Setenv("OIDC_ISSUER", "https://auth.example.com/") // trailing slash trimmed
	t.Setenv("MCP_RESOURCE", "https://polar.example.com/mcp")
	t.Setenv("OIDC_INTROSPECTION_CLIENT_ID", "polar-mcp-rs")
	t.Setenv("OIDC_INTROSPECTION_CLIENT_SECRET", "secret")
	t.Setenv("AUTH_ALLOWED_CLIENT_IDS", "claude, ")
	t.Setenv("AUTH_ALLOWED_SUBJECTS", "abc-123")
	t.Setenv("AUTH_ALLOWED_ORIGINS", "https://claude.ai")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o := cfg.OAuth
	if !o.Enabled {
		t.Fatal("OAuth should be enabled")
	}
	if o.Issuer != "https://auth.example.com" {
		t.Errorf("issuer trailing slash not trimmed: %q", o.Issuer)
	}
	if len(o.AllowedClientIDs) != 1 || o.AllowedClientIDs[0] != "claude" {
		t.Errorf("AllowedClientIDs: want [claude], got %v", o.AllowedClientIDs)
	}
	if len(o.AllowedSubjects) != 1 || o.AllowedSubjects[0] != "abc-123" {
		t.Errorf("AllowedSubjects: got %v", o.AllowedSubjects)
	}
	if len(o.AllowedGroups) != 0 {
		t.Errorf("AllowedGroups should be empty, got %v", o.AllowedGroups)
	}
}

func TestLoad_OAuthAllowsLoopbackHTTP(t *testing.T) {
	clearConfigEnv(t)
	withCreds(t)
	t.Setenv("OIDC_ISSUER", "http://127.0.0.1:9091")
	t.Setenv("MCP_RESOURCE", "http://127.0.0.1:8080/mcp")
	t.Setenv("OIDC_INTROSPECTION_CLIENT_ID", "rs")
	t.Setenv("OIDC_INTROSPECTION_CLIENT_SECRET", "s")
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
