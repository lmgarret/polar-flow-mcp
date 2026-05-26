package flow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lm/polar-flow-mcp/internal/flow/gen"
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

	c := &Client{
		cfg:    cfg,
		logger: cfg.Logger,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
			Jar:     jar,
		},
	}

	// Cold start: if we have no FLOW_SESSION at all, run a full login (or fail
	// closed if credentials aren't supplied).
	if !c.haveFlowSession() {
		if cfg.Email == "" || cfg.Password == "" {
			return nil, ErrNotLinked
		}
		c.logger.Info("flow: no cookie jar — running full login")
		if err := fullLogin(ctx, c.httpClient, cfg.Email, cfg.Password); err != nil {
			return nil, err
		}
		_ = c.persistJar()
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

// Do implements ogen-go/ogen/http.Client.
func (t *transport) Do(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet && strings.HasPrefix(req.URL.Path, "/api/") {
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
	return t.c.httpClient.Do(retry)
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
