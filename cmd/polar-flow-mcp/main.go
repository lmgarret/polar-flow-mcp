// polar-flow-mcp is the MCP server that exposes Polar Flow web API tools.
//
// Single-user mode: the server logs into one Polar Flow account
// (POLAR_EMAIL / POLAR_PASSWORD) and acts as that user for every MCP request.
// To serve multiple accounts, run multiple instances.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lmgarret/polar-flow-mcp/internal/auth"
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

	// Warm up the Polar session in the background. Login is deferred out of the
	// startup path (so the transport serves the MCP handshake immediately); this
	// gets it ready before the first tool call and surfaces credential problems
	// early, without blocking the listener.
	go func() {
		if err := flowClient.EnsureSession(ctx); err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				// Shutting down before warm-up finished — nothing to report.
			case errors.Is(err, flow.ErrLoginFailed):
				slog.Error("flow: warm-up login failed — credentials rejected? check POLAR_EMAIL / POLAR_PASSWORD", "error", err)
			default:
				slog.Warn("flow: warm-up login failed — will retry on first request", "error", err)
			}
		}
	}()

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
	if !cfg.OAuth.Enabled && cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "::1" {
		slog.Warn(
			"BIND_ADDRESS is not localhost and inbound OAuth is disabled — /mcp is unauthenticated; "+
				"set OIDC_ISSUER to enable OAuth or front the server with a trusted proxy",
			"bind_address", cfg.BindAddress,
		)
	}

	httpMCPServer := server.NewStreamableHTTPServer(mcpServer)

	// Wrap the MCP handler with the OAuth Resource Server middleware when
	// configured. When disabled this is the bare handler (historical behaviour).
	var mcpHandler http.Handler = httpMCPServer
	mux := http.NewServeMux()
	if cfg.OAuth.Enabled {
		authn := auth.New(auth.Config{
			Issuer:                    cfg.OAuth.Issuer,
			Resource:                  cfg.OAuth.Resource,
			IntrospectionClientID:     cfg.OAuth.IntrospectionClientID,
			IntrospectionClientSecret: cfg.OAuth.IntrospectionClientSecret,
			AllowedClientIDs:          cfg.OAuth.AllowedClientIDs,
			AllowedAudiences:          cfg.OAuth.AllowedAudiences,
			AllowedSubjects:           cfg.OAuth.AllowedSubjects,
			AllowedGroups:             cfg.OAuth.AllowedGroups,
			AllowedOrigins:            cfg.OAuth.AllowedOrigins,
			Logger:                    slog.Default(),
		})
		mcpHandler = authn.OriginGuard(authn.Middleware(httpMCPServer))
		mux.Handle("GET "+auth.MetadataPath, authn.MetadataHandler())
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)

	srv := &http.Server{
		Addr:    cfg.BindAddress + ":" + cfg.Port,
		Handler: withHTTPLogging(mux),
		// No WriteTimeout: the streamable-HTTP transport holds GET requests open
		// as long-lived SSE streams for server→client notifications, and a
		// WriteTimeout would force-close them on a fixed interval. ReadHeaderTimeout
		// still guards against slow-header (slowloris) clients.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
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

// withHTTPLogging logs one line per HTTP request reaching the server: the HTTP
// method, path, the JSON-RPC method carried in the body (e.g. "initialize",
// "tools/call"), the response status, and the duration. This is the access log
// that makes transport-level problems — a handshake that never gets answered, a
// request that arrives before the listener is ready — visible at a glance.
// Health-probe traffic on /healthz is logged at debug to avoid drowning it out.
func withHTTPLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rpc := rpcMethod(r)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		level := slog.LevelInfo
		if r.URL.Path == "/healthz" {
			level = slog.LevelDebug
		}
		slog.Log(r.Context(), level, "http: request",
			"method", r.Method,
			"path", r.URL.Path,
			"rpc_method", rpc,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
			"session", r.Header.Get("Mcp-Session-Id"),
		)
	})
}

// rpcMethod peeks the JSON-RPC method from a POST body without consuming it: it
// buffers the body and restores it for the next handler. Returns "" for non-POST
// requests or bodies that aren't a JSON-RPC message.
func rpcMethod(r *http.Request) string {
	if r.Method != http.MethodPost || r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var msg struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &msg)
	return msg.Method
}

// statusWriter wraps http.ResponseWriter to capture the response status code,
// while preserving the http.Flusher the streamable-HTTP SSE transport relies on.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer so SSE streaming keeps working.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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
