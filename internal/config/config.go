// Package config provides configuration loading and validation from environment variables.
// Load() is fail-closed: the server refuses to start unless all required fields are present
// and valid.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
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

	return cfg, nil
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
}
