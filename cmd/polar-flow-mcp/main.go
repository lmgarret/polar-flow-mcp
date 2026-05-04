package main

import (
	"log/slog"
	"os"

	"github.com/lm/polar-flow-mcp/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}

	config.LogStartupBanner(cfg)

	if cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "::1" {
		slog.Warn(
			"BIND_ADDRESS is not localhost — ensure PROXY_SHARED_SECRET is set "+
				"and the server is not directly reachable without the reverse proxy",
			"bind_address", cfg.BindAddress,
		)
	}

	slog.Info("server ready", "bind_address", cfg.BindAddress)

	// Plan 04 replaces this with the real HTTP server.
	select {}
}
