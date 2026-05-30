// polar-flow-mcp is the MCP server that exposes Polar Flow web API tools.
//
// Single-user mode: the server logs into one Polar Flow account
// (POLAR_EMAIL / POLAR_PASSWORD) and acts as that user for every MCP request.
// To serve multiple accounts, run multiple instances.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lmgarret/polar-flow-mcp/internal/config"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
	"github.com/lmgarret/polar-flow-mcp/internal/mcp"
)

func main() {
	initLogging()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
	config.LogStartupBanner(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	flowClient, err := flow.New(ctx, flow.Config{
		Email:    cfg.PolarEmail,
		Password: cfg.PolarPassword,
		JarPath:  cfg.CookieJarPath,
		Logger:   slog.Default(),
	})
	if err != nil {
		if errors.Is(err, flow.ErrLoginFailed) {
			slog.Error("Polar credentials rejected — check POLAR_EMAIL / POLAR_PASSWORD", "error", err)
		} else {
			slog.Error("failed to initialise Polar Flow client", "error", err)
		}
		os.Exit(1)
	}
	defer func() { _ = flowClient.Close() }()

	mcpServer := server.NewMCPServer(
		"polar-flow-mcp",
		"0.2.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, false),
	)
	mcp.RegisterTools(mcpServer, flowClient)
	mcp.RegisterResources(mcpServer)

	switch cfg.Transport {
	case "stdio":
		runStdio(ctx, mcpServer)
	case "http":
		runHTTP(ctx, cfg, mcpServer)
	default:
		slog.Error("unsupported transport", "transport", cfg.Transport)
		os.Exit(1)
	}
}

func runStdio(ctx context.Context, mcpServer *server.MCPServer) {
	slog.Info("stdio transport active")
	stdioSrv := server.NewStdioServer(mcpServer)
	if err := stdioSrv.Listen(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("stdio server error", "error", err)
		os.Exit(1)
	}
}

func runHTTP(ctx context.Context, cfg *config.Config, mcpServer *server.MCPServer) {
	if cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "::1" {
		slog.Warn(
			"BIND_ADDRESS is not localhost — single-user MCP server is not designed for "+
				"unprotected exposure; put it behind a trusted proxy",
			"bind_address", cfg.BindAddress,
		)
	}

	httpMCPServer := server.NewStreamableHTTPServer(mcpServer)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})
	mux.Handle("/mcp", httpMCPServer)
	mux.Handle("/mcp/", httpMCPServer)

	srv := &http.Server{
		Addr:         cfg.BindAddress + ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	slog.Info("server listening", "addr", srv.Addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

// initLogging loads .env then configures slog. Must run before config.Load().
func initLogging() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Error("failed to parse .env", "error", err)
		os.Exit(1)
	}
	if err := setupLogging(); err != nil {
		slog.Error("failed to set up logging", "error", err)
		os.Exit(1)
	}
	if logFile := os.Getenv("LOG_FILE"); logFile != "" {
		slog.Info("logging to file", "path", logFile)
	}
}

// setupLogging configures the default slog handler based on LOG_LEVEL and LOG_FILE.
func setupLogging() error {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	out := os.Stderr
	if logFile := os.Getenv("LOG_FILE"); logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("open log file %q: %w", logFile, err)
		}
		out = f
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})))
	return nil
}
