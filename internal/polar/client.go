// Package polar provides a typed HTTP client for the Polar AccessLink v3 API.
package polar

import (
	"net/http"
	"time"
)

// Client is a typed HTTP client for the Polar AccessLink v3 API.
// Created fresh per tool invocation; never cached at server or session level.
type Client struct {
	httpClient  *http.Client
	bearerToken string
}

// NewClient creates a new Polar API client for the given bearer token.
func NewClient(bearerToken string) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		bearerToken: bearerToken,
	}
}
