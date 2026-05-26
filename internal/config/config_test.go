// Package config_test tests fail-closed validation in config.Load().
package config_test

import (
	"strings"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/config"
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
	} {
		t.Setenv(key, "")
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
