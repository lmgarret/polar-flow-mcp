package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/config"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/mcp"
	"github.com/lm/polar-flow-mcp/internal/oauth"
	"github.com/lm/polar-flow-mcp/internal/store"
)

func main() {
	// Load .env if present; ignore missing file, fail on parse errors.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Error("failed to parse .env", "error", err)
		os.Exit(1)
	}

	// 1. Load and validate config (fail-closed, exits 1 on error).
	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}

	// 2. Emit startup security banner.
	config.LogStartupBanner(cfg)

	// 3. Open SQLite store (WAL dual-pool + migrations).
	slog.Info("DEBUG: opening store", "path", cfg.DatabasePath)
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	slog.Info("DEBUG: store opened ok")
	// Build the encryption cipher from the validated 32-byte key (cfg.EncryptionKey is raw bytes).
	cipher := crypto.NewCipher(crypto.NewBytesKeyProvider(cfg.EncryptionKey))

	// 4. Create MCP server.
	mcpServer := server.NewMCPServer(
		"polar-flow-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)
	mcp.RegisterTools(mcpServer, st, cipher)

	// 5. Branch on transport mode.
	switch cfg.Transport {
	case "stdio":
		slog.Info("stdio transport active — single-user mode", "user_id", cfg.StdioUserID)

		stdioSrv := server.NewStdioServer(mcpServer)
		stdioSrv.SetContextFunc(func(ctx context.Context) context.Context {
			return context.WithValue(ctx, auth.UserIDKey, cfg.StdioUserID)
		})

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		if err := stdioSrv.Listen(ctx, os.Stdin, os.Stdout); err != nil {
			slog.Error("stdio server error", "error", err)
			os.Exit(1)
		}

	case "http":
		// Warn on non-loopback bind address (SERV-05).
		if cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "::1" {
			slog.Warn(
				"BIND_ADDRESS is not localhost — ensure PROXY_SHARED_SECRET is set "+
					"and the server is not directly reachable without the reverse proxy",
				"bind_address", cfg.BindAddress,
			)
		}

		// Identity is injected by auth.Middleware into r.Context() before this fires.
		// WithHTTPContextFunc re-extracts the identity so MCP tool handlers receive it too.
		// This is defense-in-depth; auth.Middleware is the authoritative injection point (D-01).
		httpMCPServer := server.NewStreamableHTTPServer(mcpServer,
			server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
				// Re-extract from request context so MCP tool handlers receive it.
				// A missing identity here is a wiring bug — panic loudly rather than silently
				// serving the request without an authenticated identity (CR-02).
				id, ok := auth.UserIDFromContext(r.Context())
				if !ok {
					panic("WithHTTPContextFunc: identity not in request context; auth middleware not applied")
				}
				return context.WithValue(ctx, auth.UserIDKey, id)
			}),
		)

		// Build HTTP mux.
		mux := http.NewServeMux()

		// /healthz — no auth, always 200 (SERV-01).
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "ok")
		})

		// /readyz — no auth, checks config + DB (SERV-02).
		mux.HandleFunc("GET /readyz", readyzHandler(cfg, st))

		// /mcp — auth middleware wraps StreamableHTTPServer (SERV-03, SERV-04).
		mux.Handle("/mcp", auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader, httpMCPServer))
		mux.Handle("/mcp/", auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader, httpMCPServer))

		// /oauth — auth middleware wraps OAuth handlers.
		oauthHandlers := oauth.NewHandlers(cfg, st, cipher)
		mux.Handle("GET /oauth/login",
			auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader,
				http.HandlerFunc(oauthHandlers.Login)))
		mux.Handle("GET /oauth/callback",
			auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader,
				http.HandlerFunc(oauthHandlers.Callback)))

		// Start HTTP server with graceful shutdown.
		srv := &http.Server{
			Addr:         cfg.BindAddress + ":" + cfg.Port,
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		slog.Info("DEBUG: reached listen")
		slog.Info("server listening", "addr", srv.Addr)

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
		}()

		<-quit
		slog.Info("shutting down")

		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}
}

// readyzHandler returns an http.HandlerFunc that checks four readiness conditions and
// responds with a JSON body listing each check's name, status, and optional error.
// Returns 200 when all checks pass, 503 when any check fails (SERV-02).
func readyzHandler(cfg *config.Config, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type check struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		}

		checks := make([]check, 0, 4)
		allOK := true

		// Check 1: AUTH_PROXY not unconfigured.
		if cfg.AuthProxy == "unconfigured" || cfg.AuthProxy == "" {
			checks = append(checks, check{Name: "auth_proxy", Status: "fail", Error: "AUTH_PROXY not configured"})
			allOK = false
		} else {
			checks = append(checks, check{Name: "auth_proxy", Status: "ok"})
		}

		// Check 2: PROXY_SHARED_SECRET set.
		if cfg.ProxySharedSecret == "" {
			checks = append(checks, check{Name: "proxy_secret", Status: "fail", Error: "PROXY_SHARED_SECRET not set"})
			allOK = false
		} else {
			checks = append(checks, check{Name: "proxy_secret", Status: "ok"})
		}

		// Check 3: DB reachable (both pools — D-07).
		if err := st.Ping(r.Context()); err != nil {
			checks = append(checks, check{Name: "database", Status: "fail", Error: err.Error()})
			allOK = false
		} else {
			checks = append(checks, check{Name: "database", Status: "ok"})
		}

		// Check 4: Encryption key loaded and correct length.
		if len(cfg.EncryptionKey) != 32 {
			checks = append(checks, check{Name: "encryption_key", Status: "fail", Error: "encryption key not loaded or wrong length"})
			allOK = false
		} else {
			checks = append(checks, check{Name: "encryption_key", Status: "ok"})
		}

		w.Header().Set("Content-Type", "application/json")
		if allOK {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"checks": checks})
	}
}
