package flow

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// NewForTesting returns a Client whose API calls go to baseURL through a
// plain http.Client, with no login, refresh or cookie persistence. It exists
// so packages that wrap Client (internal/mcp) can run their handlers against
// an httptest server that fakes Flow's wire shapes. Not for production use.
func NewForTesting(baseURL string) (*Client, error) {
	jar := newCookieJar()
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("flow: parse test base URL: %w", err)
	}
	c := &Client{
		sleepURL:   u,
		logger:     slog.Default(),
		httpClient: &http.Client{Jar: jar},
	}
	api, err := gen.NewClient(baseURL, (*securitySource)(c), gen.WithClient(c.httpClient))
	if err != nil {
		return nil, fmt.Errorf("flow: build test client: %w", err)
	}
	c.API = api
	return c, nil
}
