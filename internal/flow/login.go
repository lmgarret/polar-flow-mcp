package flow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Browser-ish User-Agent — observed in auth.md as required (Polar serves
// different responses to python-requests-style UAs on adjacent endpoints).
const defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) polar-flow-mcp/1.0"

// fullLogin runs the 9-step login chain from auth.md against auth.polar.com +
// flow.polar.com using the supplied http.Client (which must have a CookieJar).
// On success the jar holds FLOW_SESSION + PLAY_SESSION_FLOW (flow.polar.com)
// and session_id + remember-me (auth.polar.com).
func fullLogin(ctx context.Context, client *http.Client, email, password string) error {
	if client.Jar == nil {
		return fmt.Errorf("flow: http.Client must have a CookieJar for login")
	}

	// 1. GET flow.polar.com/login — bootstraps PLAY_SESSION_FLOW.
	if _, err := doGET(ctx, client, "https://flow.polar.com/login", nil); err != nil {
		return fmt.Errorf("flow: step 1 (GET /login): %w", err)
	}

	// 2. Read PLAY_SESSION_FLOW from jar and extract csrfToken from its `data`.
	csrfToken, err := readPlayCsrfToken(client)
	if err != nil {
		return fmt.Errorf("flow: step 2 (extract csrfToken): %w", err)
	}

	// 3-6. GET /flowSso/login bounces through auth.polar.com/oauth/authorize and
	//      lands on auth.polar.com/login. http.Client follows the redirect
	//      chain for us. The login page is now a JS SPA (no server-rendered
	//      _csrf hidden input), but auth.polar.com still sets an XSRF-TOKEN
	//      cookie on every GET, and the SPA submits its value as the _csrf
	//      form field — textbook double-submit-cookie CSRF. So we just read
	//      the cookie from the jar instead of scraping the (no-longer-present)
	//      hidden input.
	loginURL := "https://flow.polar.com/flowSso/login?" + url.Values{
		"csrfToken": {csrfToken},
		"returnUrl": {"/"},
	}.Encode()
	if _, err := doGET(ctx, client, loginURL, http.Header{"Accept": {"text/html"}}); err != nil {
		return fmt.Errorf("flow: step 3-6 (bounce to auth login): %w", err)
	}
	formCsrf, err := readCookie(client, "https://auth.polar.com", "XSRF-TOKEN")
	if err != nil {
		return fmt.Errorf("flow: step 6 (read XSRF-TOKEN cookie): %w", err)
	}

	// 7. POST /login with credentials. http.Client will follow the post-auth
	//    redirect chain (steps 8-9) and land on flow.polar.com — by the time
	//    Do() returns, FLOW_SESSION is in the jar.
	form := url.Values{
		"_csrf":       {formCsrf},
		"username":    {email},
		"password":    {password},
		"remember-me": {"on"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://auth.polar.com/login", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("flow: build login POST: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://auth.polar.com")
	req.Header.Set("Referer", "https://auth.polar.com/login")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("flow: POST /login: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	// 9. Final check: do we have a FLOW_SESSION on flow.polar.com?
	if !jarHas(client, "https://flow.polar.com", "FLOW_SESSION") {
		// If the credentials were wrong the login form would re-render here
		// rather than redirecting back to flow.polar.com.
		return fmt.Errorf("%w: no FLOW_SESSION after login chain", ErrLoginFailed)
	}
	return nil
}

// silentRefresh runs the 3-hop refresh from auth.md when FLOW_SESSION has
// expired but session_id / remember-me on auth.polar.com are still valid.
func silentRefresh(ctx context.Context, client *http.Client) error {
	csrfToken, err := readPlayCsrfToken(client)
	if err != nil {
		return fmt.Errorf("flow: silent refresh: no csrfToken in PLAY_SESSION_FLOW: %w", err)
	}
	loginURL := "https://flow.polar.com/flowSso/login?" + url.Values{
		"csrfToken": {csrfToken},
		"returnUrl": {"/"},
	}.Encode()
	if _, err := doGET(ctx, client, loginURL, http.Header{"Accept": {"text/html"}}); err != nil {
		return fmt.Errorf("flow: silent refresh: %w", err)
	}
	if !jarHas(client, "https://flow.polar.com", "FLOW_SESSION") {
		return fmt.Errorf("%w: silent refresh did not yield FLOW_SESSION", ErrLoginFailed)
	}
	return nil
}

// doGET issues a GET with a browser UA and returns the body bytes (limited to 1MiB).
func doGET(ctx context.Context, client *http.Client, u string, extraHdr http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	for k, vs := range extraHdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func jarHas(client *http.Client, rawURL, name string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == name && c.Value != "" {
			return true
		}
	}
	return false
}

func readPlayCsrfToken(client *http.Client) (string, error) {
	u, _ := url.Parse("https://flow.polar.com")
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "PLAY_SESSION_FLOW" && c.Value != "" {
			return extractPlayCsrfToken(c.Value)
		}
	}
	return "", fmt.Errorf("flow: PLAY_SESSION_FLOW not in cookie jar")
}

// readCookie returns the value of the named cookie on the given URL's domain,
// or an error if absent.
func readCookie(client *http.Client, rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == name && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("flow: cookie %q not present for %s", name, u.Host)
}
