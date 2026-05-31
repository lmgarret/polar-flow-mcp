package flow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Config holds the inputs the Flow client needs at construction time.
type Config struct {
	Email       string // POLAR_EMAIL
	Password    string // POLAR_PASSWORD
	JarPath     string // COOKIE_JAR_PATH — chmod-600 JSON file
	HTTPTimeout time.Duration
	Logger      *slog.Logger
}

// Client is the high-level Polar Flow API client.
// One per process — it serializes auth state under a mutex.
type Client struct {
	cfg    Config
	logger *slog.Logger

	// httpClient is shared between login (which needs to follow redirect chains)
	// and ogen-driven requests (via Transport). The jar is owned here so both
	// paths see the same cookies.
	httpClient *http.Client

	// session-state mutex protects flowSession and serializes refresh/login.
	mu          sync.Mutex
	refreshing  bool
	lastRefresh time.Time

	// API is the generated ogen client. Use it for raw access if a method on
	// Client doesn't cover an operation.
	API *gen.Client
}

// New constructs a Client and ensures we have a usable session (loads jar from
// disk, runs login if needed). Returns ErrNotLinked if no credentials are
// supplied AND no jar exists.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	jar := newCookieJar()
	persisted, err := loadCookieJarFile(cfg.JarPath)
	if err != nil {
		return nil, err
	}
	_ = seedJar(jar, persisted)

	tlsTransport, txErr := newBrowserTransport()
	if txErr != nil {
		return nil, fmt.Errorf("flow: build TLS transport: %w", txErr)
	}
	c := &Client{
		cfg:    cfg,
		logger: cfg.Logger,
		httpClient: &http.Client{
			Timeout:   cfg.HTTPTimeout,
			Jar:       jar,
			Transport: tlsTransport,
		},
	}

	// Cold start is non-blocking: we do NOT log in here. A full login can take
	// several seconds (CloudFront WAF + redirect chain), and blocking New blocks
	// the HTTP listener from binding — which races the MCP client's initialize
	// handshake and makes it time out. Instead login is deferred to the first
	// request (EnsureSession, called from transport.Do) and can be warmed up in
	// the background by the caller. We fail closed here only when there is no way
	// to ever authenticate: no persisted session AND no credentials.
	if !c.haveFlowSession() && (cfg.Email == "" || cfg.Password == "") {
		return nil, ErrNotLinked
	}
	if c.haveFlowSession() {
		c.logger.Info("flow: reusing persisted session from cookie jar")
	} else {
		c.logger.Info("flow: no session yet — login deferred to first request")
	}

	// Build ogen client. Transport handles X-Requested-With + 401 retry.
	api, err := gen.NewClient(
		"https://flow.polar.com",
		(*securitySource)(c),
		gen.WithClient(&transport{c: c}),
	)
	if err != nil {
		return nil, fmt.Errorf("flow: build gen client: %w", err)
	}
	c.API = api
	return c, nil
}

// EnsureSession guarantees the jar holds a FLOW_SESSION cookie, running a full
// login on a cold start (empty jar). It is safe for concurrent use and for a
// background warm-up goroutine: the login is serialized under mu and coalesced,
// so it runs at most once even if several requests race in at startup.
//
// Login is deferred to here (rather than done in New) so process startup stays
// instant and the HTTP listener binds before the MCP client's initialize
// handshake arrives. The 401-retry path in transport.Do still handles the
// separate case of an expired session via refresh.
func (c *Client) EnsureSession(ctx context.Context) error {
	if c.flowSessionValue() != "" {
		return nil // fast path: already authenticated, no lock needed.
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.flowSessionValue() != "" {
		return nil // another goroutine logged in while we waited for the lock.
	}
	if c.cfg.Email == "" || c.cfg.Password == "" {
		return ErrNotLinked
	}
	c.logger.Info("flow: no session — running full login")
	start := time.Now()
	if err := fullLogin(ctx, c.httpClient, c.cfg.Email, c.cfg.Password); err != nil {
		return err
	}
	c.lastRefresh = time.Now()
	_ = c.persistJar()
	c.logger.Info("flow: login complete", "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// haveFlowSession returns true if the jar holds a non-empty FLOW_SESSION cookie.
func (c *Client) haveFlowSession() bool {
	return jarHas(c.httpClient, "https://flow.polar.com", "FLOW_SESSION")
}

// flowSessionValue reads the current FLOW_SESSION cookie value, or "" if absent.
func (c *Client) flowSessionValue() string {
	u, _ := url.Parse("https://flow.polar.com")
	for _, ck := range c.httpClient.Jar.Cookies(u) {
		if ck.Name == "FLOW_SESSION" {
			return ck.Value
		}
	}
	return ""
}

// persistJar snapshots the in-memory jar to disk (chmod 600). Best-effort —
// failures are logged but not returned to API callers.
func (c *Client) persistJar() error {
	snap := snapshotJar(c.httpClient.Jar)
	if err := saveCookieJarFile(c.cfg.JarPath, snap); err != nil {
		c.logger.Warn("flow: persist cookie jar failed", "err", err, "path", c.cfg.JarPath)
		return err
	}
	return nil
}

// refresh runs the 3-hop silent refresh, falling back to a full login if it
// fails (e.g. session_id expired beyond remember-me's reach). Serialized by mu.
func (c *Client) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Coalesce concurrent refresh attempts: if one ran in the last 5s, trust it.
	if time.Since(c.lastRefresh) < 5*time.Second {
		return nil
	}
	c.refreshing = true
	defer func() { c.refreshing = false }()

	c.logger.Debug("flow: silent refresh starting")
	if err := silentRefresh(ctx, c.httpClient); err != nil {
		c.logger.Info("flow: silent refresh failed — attempting full login", "err", err)
		if c.cfg.Email == "" || c.cfg.Password == "" {
			return fmt.Errorf("%w (no credentials to fall back to)", err)
		}
		if err := fullLogin(ctx, c.httpClient, c.cfg.Email, c.cfg.Password); err != nil {
			return err
		}
	}
	c.lastRefresh = time.Now()
	_ = c.persistJar()
	return nil
}

// securitySource is an adapter that lets *Client implement gen.SecuritySource
// without exposing FlowSession-related methods publicly.
type securitySource Client

func (s *securitySource) SessionCookie(_ context.Context, _ gen.OperationName) (gen.SessionCookie, error) {
	c := (*Client)(s)
	return gen.SessionCookie{APIKey: c.flowSessionValue()}, nil
}

// transport is the ht.Client adapter passed to gen.WithClient. It owns the
// X-Requested-With injection and the once-on-401 retry.
type transport struct {
	c *Client
}

// primeSession ensures a session exists before the first API call, running the
// deferred cold-start login at most once. ogen's SecuritySource attached an
// empty FLOW_SESSION when it built this request (we had no session then), so it
// swaps the fresh value onto req. No-op once authenticated.
func (t *transport) primeSession(req *http.Request) error {
	if t.c.flowSessionValue() != "" {
		return nil
	}
	if err := t.c.EnsureSession(req.Context()); err != nil {
		return err
	}
	stripCookie(req, "FLOW_SESSION")
	if v := t.c.flowSessionValue(); v != "" {
		req.AddCookie(&http.Cookie{Name: "FLOW_SESSION", Value: v})
	}
	return nil
}

// Do implements ogen-go/ogen/http.Client.
func (t *transport) Do(req *http.Request) (*http.Response, error) {
	// Cold start: log in lazily on the first real API call (deferred out of New
	// so the listener binds instantly).
	if err := t.primeSession(req); err != nil {
		return nil, err
	}

	// Play's CSRF filter requires X-Requested-With on every mutation, not just
	// /api/* — DELETE /training/target/{id} 403s without it.
	if req.Method != http.MethodGet {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	// Preserve body for retry — ogen always passes a seekable bytes reader, but
	// belt-and-braces in case future versions pass a streaming body.
	var bodyBytes []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("flow: buffer request body: %w", err)
		}
		_ = req.Body.Close()
		bodyBytes = b
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	resp, err := t.c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	slog.Debug("flow: http", "method", req.Method, "path", req.URL.Path, "status", resp.StatusCode)
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// 401 path: peek the body to decide whether to refresh.
	peek, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if !isNotAuthenticatedBody(peek) {
		// Not a session-expiry 401 — surface the original response (with body restored).
		resp.Body = io.NopCloser(bytes.NewReader(peek))
		return resp, nil
	}
	if rerr := t.c.refresh(req.Context()); rerr != nil {
		// Refresh failed — bubble up; restore body for the caller.
		resp.Body = io.NopCloser(bytes.NewReader(peek))
		return resp, nil
	}

	// Build a retry request. The cookie jar already has the new FLOW_SESSION,
	// but ogen added the OLD value via SecuritySource onto the original
	// req.Header. Strip stale FLOW_SESSION cookies and let the jar re-attach
	// the fresh one.
	retry := req.Clone(req.Context())
	if bodyBytes != nil {
		retry.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	stripCookie(retry, "FLOW_SESSION")
	if v := t.c.flowSessionValue(); v != "" {
		retry.AddCookie(&http.Cookie{Name: "FLOW_SESSION", Value: v})
	}
	retryResp, retryErr := t.c.httpClient.Do(retry)
	if retryErr == nil {
		slog.Debug("flow: http retry", "method", retry.Method, "path", retry.URL.Path, "status", retryResp.StatusCode)
	}
	return retryResp, retryErr
}

// stripCookie removes any cookie with the given name from r.Header["Cookie"].
func stripCookie(r *http.Request, name string) {
	existing := r.Cookies()
	r.Header.Del("Cookie")
	for _, c := range existing {
		if c.Name == name {
			continue
		}
		r.AddCookie(c)
	}
}

// Close persists the cookie jar. Safe to call multiple times.
func (c *Client) Close() error {
	return c.persistJar()
}
