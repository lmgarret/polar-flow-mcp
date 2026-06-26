package auth

import (
	"net"
	"net/http"
	"strings"
)

// identity is the forward-auth-asserted caller on the /authorize step.
type identity struct {
	Email  string
	Groups []string
}

// forwardAuthIdentity extracts the authenticated identity from the forward-auth
// headers, but ONLY when the immediate peer is a trusted proxy. If the request
// did not arrive through a trusted proxy the headers are ignored entirely —
// this is the control that stops a public caller from spoofing an identity.
// ok is false when the peer is untrusted or no email header is present.
func (a *Authenticator) forwardAuthIdentity(r *http.Request) (identity, bool) {
	if !a.peerTrusted(r.RemoteAddr) {
		a.log.Warn("forward-auth headers ignored: peer is not a trusted proxy", "remote", r.RemoteAddr)
		return identity{}, false
	}
	email := strings.TrimSpace(r.Header.Get(a.cfg.EmailHeader))
	if email == "" {
		return identity{}, false
	}
	return identity{Email: email, Groups: parseGroups(r.Header.Get(a.cfg.GroupsHeader))}, true
}

// peerTrusted reports whether remoteAddr's IP falls within a trusted-proxy net.
func (a *Authenticator) peerTrusted(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range a.cfg.TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// parseGroups splits a comma-separated forward-auth groups header.
func parseGroups(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
