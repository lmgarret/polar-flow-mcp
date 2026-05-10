// Package polar provides a typed HTTP client for the Polar AccessLink v3 API.
package polar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

//nolint:gochecknoglobals
var (
	// tokenEndpoint is the Polar OAuth2 token exchange URL. Overridable in tests.
	tokenEndpoint = "https://polarremote.com/v2/oauth2/token"

	// registerEndpoint is the Polar AccessLink user registration URL. Overridable in tests.
	registerEndpoint = "https://www.polaraccesslink.com/v3/users"

	// defaultHTTPClient is shared by package-level functions (ExchangeCode, RegisterUser).
	defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// TokenResponse holds the fields returned by the Polar token endpoint.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	XUserID     int64  `json:"x_user_id"`
}

// ExchangeCode exchanges a Polar authorization code for an access token (per OAUTH-03).
// Uses HTTP Basic auth with client credentials, form-encoded body. Caller-supplied ctx
// satisfies the noctx linter via http.NewRequestWithContext.
func ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURL string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("polar: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polar: token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("polar: token exchange: status %d: %s", resp.StatusCode, body)
	}
	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("polar: decode token response: %w", err)
	}
	return &tr, nil
}

// RegisterUser registers the access token's owner with Polar AccessLink (per OAUTH-04).
// HTTP 409 indicates the user is already registered; treat as idempotent success and
// return (0, nil). Callers MUST use the x_user_id from TokenResponse as the authoritative
// polar_user_id in all cases (200 and 409) — the 409 body does not contain the user object.
func RegisterUser(ctx context.Context, accessToken string) (int64, error) {
	body, _ := json.Marshal(map[string]string{"member-id": accessToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerEndpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("polar: build register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("polar: register user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusConflict {
		return 0, nil
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("polar: register user: status %d: %s", resp.StatusCode, errBody)
	}
	var reg struct {
		PolarUserID int64 `json:"polar-user-id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return 0, fmt.Errorf("polar: decode register response: %w", err)
	}
	return reg.PolarUserID, nil
}
