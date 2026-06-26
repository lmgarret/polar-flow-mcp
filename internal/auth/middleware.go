package auth

import (
	"context"
	"net/http"
	"strings"
)

// identityKey is an unexported context key (prevents cross-package collision).
type identityKey struct{}

// Identity is the validated caller, stashed in the request context after a
// successful token check.
type Identity struct {
	Subject string
	Scope   string
}

// IdentityFrom returns the validated Identity from ctx, if any.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// Middleware wraps a handler, requiring a valid Bearer access token on every
// request. Missing/invalid/expired tokens get 401 with a WWW-Authenticate
// header that points clients at the protected-resource metadata. It is
// fail-closed: any verification error is a 401.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			a.challenge(w, "missing bearer token")
			return
		}
		c, err := a.parse(token)
		if err != nil {
			a.log.Warn("access token rejected", "error", err)
			a.challenge(w, "invalid token")
			return
		}
		// Verify this is an access token minted by us, for us, for an allowed
		// subject. The audience check is the confused-deputy defence: a token
		// for some other resource is refused even though we signed it.
		if c.Typ != typAccess ||
			c.Issuer != a.cfg.Issuer ||
			!contains(c.Audience, a.cfg.Resource) ||
			!a.allowedEmail(c.Subject) {
			a.log.Warn("access token failed claim checks", "sub", c.Subject, "aud", []string(c.Audience))
			a.challenge(w, "token not accepted by this server")
			return
		}

		ctx := context.WithValue(r.Context(), identityKey{}, Identity{
			Subject: c.Subject,
			Scope:   c.Scope,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OriginGuard enforces the Origin allowlist (DNS-rebinding defence). It is a
// no-op unless AllowedOrigins is configured. Requests with no Origin header are
// always allowed (server-to-server callers such as Claude's backend omit it).
func (a *Authenticator) OriginGuard(next http.Handler) http.Handler {
	if len(a.cfg.AllowedOrigins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !contains(a.cfg.AllowedOrigins, origin) {
			a.log.Warn("request rejected by Origin allowlist", "origin", origin)
			http.Error(w, "Forbidden: origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// protectedResourceMetadata is the RFC 9728 document advertised so clients can
// discover the authorization server — which, here, is this server itself.
type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// MetadataHandler serves the protected-resource metadata at MetadataPath.
func (a *Authenticator) MetadataHandler() http.Handler {
	doc := protectedResourceMetadata{
		Resource:               a.cfg.Resource,
		AuthorizationServers:   []string{a.cfg.Issuer},
		ScopesSupported:        []string{"openid", "profile", "offline_access"},
		BearerMethodsSupported: []string{"header"},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, doc)
	})
}

// challenge writes a 401 with a WWW-Authenticate header pointing at the
// protected-resource metadata, per RFC 9728 / the MCP authorization spec.
func (a *Authenticator) challenge(w http.ResponseWriter, detail string) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", resource_metadata="`+a.metadataURL()+`"`)
	http.Error(w, "Unauthorized: "+detail, http.StatusUnauthorized)
}

// metadataURL is the absolute URL of the protected-resource metadata, derived
// from the issuer host so it sits at the host root.
func (a *Authenticator) metadataURL() string {
	res := a.cfg.Issuer
	if i := strings.Index(res, "://"); i >= 0 {
		rest := res[i+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			return res[:i+3] + rest[:slash] + MetadataPath
		}
	}
	return res + MetadataPath
}

// bearerToken extracts the token from an Authorization: Bearer header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
