package flow

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	azuretls "github.com/Noooste/azuretls-client"
	fhttp "github.com/Noooste/fhttp"
)

// syncHosts are the origins whose cookies must travel between the stdlib
// http.Client jar (used by the rest of the app + ogen) and the transient
// azuretls.Session jar (used for the WAF-sensitive login chain).
var syncHosts = []string{
	"https://flow.polar.com",
	"https://auth.polar.com",
}

// defaultUserAgent is used by the stdlib http.Client for /api/* calls.
// azuretls picks its own UA from session.Browser=Chrome and ignores this.
const defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// fullLogin runs the 9-step login chain via a transient azuretls.Session
// (Chrome JA3 + Chrome HTTP/2 fingerprint). Cookies from the supplied
// http.Client.Jar are seeded into azuretls so partial-state retries work; on
// success the resulting cookies (notably FLOW_SESSION) are copied back into
// the supplied jar for the rest of the app to use.
func fullLogin(ctx context.Context, client *http.Client, email, password string) error {
	if client.Jar == nil {
		return fmt.Errorf("flow: http.Client must have a CookieJar for login")
	}
	sess := newChromeSession()
	defer sess.Close()
	copyJarStdlibToAzuretls(client.Jar, sess)

	// 1. GET flow.polar.com/login — bootstraps PLAY_SESSION_FLOW.
	if _, err := sess.Get("https://flow.polar.com/login"); err != nil {
		return fmt.Errorf("flow: step 1 (GET /login): %w", err)
	}

	// 2. Read PLAY_SESSION_FLOW from azuretls jar and extract csrfToken.
	csrfToken, err := readPlayCsrfTokenAzuretls(sess)
	if err != nil {
		return fmt.Errorf("flow: step 2 (extract csrfToken): %w", err)
	}

	// 3-6. GET /flowSso/login follows redirects through auth.polar.com/oauth/authorize
	//      and lands on auth.polar.com/login. azuretls handles the redirect chain.
	loginURL := "https://flow.polar.com/flowSso/login?" + url.Values{
		"csrfToken": {csrfToken},
		"returnUrl": {"/"},
	}.Encode()
	if _, err := sess.Get(loginURL); err != nil {
		return fmt.Errorf("flow: step 3-6 (bounce to auth login): %w", err)
	}
	formCsrf, err := readCookieAzuretls(sess, "https://auth.polar.com", "XSRF-TOKEN")
	if err != nil {
		return fmt.Errorf("flow: step 6 (read XSRF-TOKEN cookie): %w", err)
	}

	// 7. POST /login with credentials. azuretls follows the post-auth redirect
	//    chain and lands on flow.polar.com — FLOW_SESSION ends up in its jar.
	form := url.Values{
		"_csrf":       {formCsrf},
		"username":    {email},
		"password":    {password},
		"remember-me": {"on"},
	}
	resp, err := sess.Do(&azuretls.Request{
		Method: fhttp.MethodPost,
		Url:    "https://auth.polar.com/login",
		Body:   form.Encode(),
		OrderedHeaders: azuretls.OrderedHeaders{
			{"content-type", "application/x-www-form-urlencoded"},
			{"origin", "https://auth.polar.com"},
			{"referer", "https://auth.polar.com/login"},
		},
		TimeOut: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("flow: POST /login: %w", err)
	}

	// Mirror azuretls cookies back into the stdlib jar so ogen sees them.
	copyJarAzuretlsToStdlib(sess, client.Jar)

	slog.Debug("flow: POST /login chain finished",
		"final_status", resp.StatusCode,
		"final_url", resp.Url,
		"have_flow_session", jarHas(client, "https://flow.polar.com", "FLOW_SESSION"),
		"have_play_session_flow", jarHas(client, "https://flow.polar.com", "PLAY_SESSION_FLOW"),
		"have_session_id", jarHas(client, "https://auth.polar.com", "session_id"),
		"have_remember_me", jarHas(client, "https://auth.polar.com", "remember-me"),
	)

	if !jarHas(client, "https://flow.polar.com", "FLOW_SESSION") {
		return fmt.Errorf("%w: no FLOW_SESSION after login chain (final=%s, status=%d)",
			ErrLoginFailed, resp.Url, resp.StatusCode)
	}
	return nil
}

// silentRefresh runs the 3-hop refresh via azuretls when FLOW_SESSION has
// expired but session_id / remember-me on auth.polar.com are still valid.
func silentRefresh(ctx context.Context, client *http.Client) error {
	sess := newChromeSession()
	defer sess.Close()
	copyJarStdlibToAzuretls(client.Jar, sess)

	csrfToken, err := readPlayCsrfTokenAzuretls(sess)
	if err != nil {
		return fmt.Errorf("flow: silent refresh: no csrfToken in PLAY_SESSION_FLOW: %w", err)
	}
	loginURL := "https://flow.polar.com/flowSso/login?" + url.Values{
		"csrfToken": {csrfToken},
		"returnUrl": {"/"},
	}.Encode()
	if _, err := sess.Get(loginURL); err != nil {
		return fmt.Errorf("flow: silent refresh: %w", err)
	}
	copyJarAzuretlsToStdlib(sess, client.Jar)
	if !jarHas(client, "https://flow.polar.com", "FLOW_SESSION") {
		return fmt.Errorf("%w: silent refresh did not yield FLOW_SESSION", ErrLoginFailed)
	}
	return nil
}

// newChromeSession returns a fresh azuretls.Session preset to the Chrome
// browser fingerprint (matches JA3 + HTTP/2 settings + header order).
func newChromeSession() *azuretls.Session {
	sess := azuretls.NewSession()
	sess.Browser = azuretls.Chrome
	return sess
}

// jarHas returns true if the named cookie exists with a non-empty value on
// the given URL's domain in the supplied stdlib jar.
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

func readPlayCsrfTokenAzuretls(sess *azuretls.Session) (string, error) {
	u, _ := url.Parse("https://flow.polar.com")
	for _, c := range sess.CookieJar.Cookies(u) {
		if c.Name == "PLAY_SESSION_FLOW" && c.Value != "" {
			return extractPlayCsrfToken(c.Value)
		}
	}
	return "", fmt.Errorf("flow: PLAY_SESSION_FLOW not in azuretls jar")
}

func readCookieAzuretls(sess *azuretls.Session, rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	for _, c := range sess.CookieJar.Cookies(u) {
		if c.Name == name && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("flow: cookie %q not present for %s (azuretls jar)", name, u.Host)
}

// copyJarStdlibToAzuretls copies the cookies the stdlib jar would send on
// each syncHost into azuretls's jar. Stdlib's CookieJar.Cookies() only
// surfaces Name/Value (path/domain/expiry are internal), so azuretls assigns
// defaults — sufficient for transmitting them on subsequent requests.
func copyJarStdlibToAzuretls(stdJar http.CookieJar, sess *azuretls.Session) {
	for _, h := range syncHosts {
		u, _ := url.Parse(h)
		stdCookies := stdJar.Cookies(u)
		if len(stdCookies) == 0 {
			continue
		}
		fcookies := make([]*fhttp.Cookie, 0, len(stdCookies))
		for _, c := range stdCookies {
			fcookies = append(fcookies, &fhttp.Cookie{
				Name:  c.Name,
				Value: c.Value,
			})
		}
		sess.CookieJar.SetCookies(u, fcookies)
	}
}

// copyJarAzuretlsToStdlib copies cookies from azuretls into the stdlib jar.
// Same Name/Value-only fidelity.
func copyJarAzuretlsToStdlib(sess *azuretls.Session, stdJar http.CookieJar) {
	for _, h := range syncHosts {
		u, _ := url.Parse(h)
		fcookies := sess.CookieJar.Cookies(u)
		if len(fcookies) == 0 {
			continue
		}
		scookies := make([]*http.Cookie, 0, len(fcookies))
		for _, c := range fcookies {
			scookies = append(scookies, &http.Cookie{
				Name:  c.Name,
				Value: c.Value,
			})
		}
		stdJar.SetCookies(u, scookies)
	}
}
