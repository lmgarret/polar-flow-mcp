package flow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
)

// cookieJarFile is the on-disk shape of the persisted jar.
// Only the cookies that survive across processes are stored — transient
// AWSALB/timezone cookies are not load-bearing (see auth.md).
type cookieJarFile struct {
	FlowSession    string `json:"flow_session,omitempty"`     // .flow.polar.com FLOW_SESSION (~100d cookie, ~1h JWT)
	PlaySession    string `json:"play_session_flow,omitempty"` // flow.polar.com PLAY_SESSION_FLOW
	AuthSessionID  string `json:"auth_session_id,omitempty"`  // auth.polar.com session_id
	AuthRememberMe string `json:"auth_remember_me,omitempty"` // auth.polar.com remember-me (rolling 14d)
}

// loadCookieJarFile reads the jar file at path. Returns an empty jar (and no
// error) if the file does not exist.
func loadCookieJarFile(path string) (cookieJarFile, error) {
	var jar cookieJarFile
	b, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied
	if err != nil {
		if os.IsNotExist(err) {
			return jar, nil
		}
		return jar, fmt.Errorf("flow: read cookie jar %q: %w", path, err)
	}
	if len(b) == 0 {
		return jar, nil
	}
	if err := json.Unmarshal(b, &jar); err != nil {
		return jar, fmt.Errorf("flow: parse cookie jar %q: %w", path, err)
	}
	return jar, nil
}

// saveCookieJarFile writes the jar atomically (write-then-rename) with mode 0600.
func saveCookieJarFile(path string, jar cookieJarFile) error {
	b, err := json.MarshalIndent(jar, "", "  ")
	if err != nil {
		return fmt.Errorf("flow: marshal cookie jar: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("flow: write cookie jar %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("flow: rename cookie jar to %q: %w", path, err)
	}
	return nil
}

// seedJar populates an http.CookieJar with the persisted cookies, so subsequent
// http.Client requests automatically include them on the right domains.
func seedJar(jar http.CookieJar, persisted cookieJarFile) error {
	flowURL := &url.URL{Scheme: "https", Host: "flow.polar.com"}
	authURL := &url.URL{Scheme: "https", Host: "auth.polar.com"}
	if persisted.FlowSession != "" {
		jar.SetCookies(flowURL, []*http.Cookie{{Name: "FLOW_SESSION", Value: persisted.FlowSession, Path: "/", Domain: "flow.polar.com"}})
	}
	if persisted.PlaySession != "" {
		jar.SetCookies(flowURL, []*http.Cookie{{Name: "PLAY_SESSION_FLOW", Value: persisted.PlaySession, Path: "/", Domain: "flow.polar.com"}})
	}
	if persisted.AuthSessionID != "" {
		jar.SetCookies(authURL, []*http.Cookie{{Name: "session_id", Value: persisted.AuthSessionID, Path: "/", Domain: "auth.polar.com"}})
	}
	if persisted.AuthRememberMe != "" {
		jar.SetCookies(authURL, []*http.Cookie{{Name: "remember-me", Value: persisted.AuthRememberMe, Path: "/", Domain: "auth.polar.com"}})
	}
	return nil
}

// snapshotJar extracts the persisted cookies from a live http.CookieJar.
func snapshotJar(jar http.CookieJar) cookieJarFile {
	flowURL := &url.URL{Scheme: "https", Host: "flow.polar.com"}
	authURL := &url.URL{Scheme: "https", Host: "auth.polar.com"}
	var f cookieJarFile
	for _, c := range jar.Cookies(flowURL) {
		switch c.Name {
		case "FLOW_SESSION":
			f.FlowSession = c.Value
		case "PLAY_SESSION_FLOW":
			f.PlaySession = c.Value
		}
	}
	for _, c := range jar.Cookies(authURL) {
		switch c.Name {
		case "session_id":
			f.AuthSessionID = c.Value
		case "remember-me":
			f.AuthRememberMe = c.Value
		}
	}
	return f
}

// newCookieJar returns an empty in-memory cookie jar. Cookiejar.New never
// returns an error when options is nil.
func newCookieJar() http.CookieJar {
	j, _ := cookiejar.New(nil)
	return j
}
