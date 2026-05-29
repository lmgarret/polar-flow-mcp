package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// MetadataPath is the RFC 9728 well-known location served at the resource host.
const MetadataPath = "/.well-known/oauth-protected-resource"

// identityKey is an unexported context key (prevents cross-package collision).
type identityKey struct{}

// Identity is the validated caller, stashed in the request context after a
// successful introspection.
type Identity struct {
	Subject  string
	ClientID string
	Scope    string
	Username string
}

// IdentityFrom returns the validated Identity from ctx, if any.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// Middleware wraps a handler, requiring a valid Bearer token on every request.
// Missing/invalid/inactive tokens get 401 with a WWW-Authenticate header that
// points clients at the protected-resource metadata; tokens that fail a
// configured pin get 403. It is fail-closed: any introspection error is a 401.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			a.challenge(w, "missing bearer token")
			return
		}

		res, err := a.introspect(r.Context(), token)
		if err != nil {
			a.log.Warn("token introspection failed", "error", err)
			a.challenge(w, "token validation failed")
			return
		}
		if !res.Active {
			a.challenge(w, "token inactive")
			return
		}
		if err := a.checkPins(res); err != nil {
			a.log.Warn("token rejected by pin",
				"error", err,
				"sub", res.Subject,
				"client_id", res.ClientID,
				"aud", []string(res.Audience),
				"groups", []string(res.Groups),
			)
			http.Error(w, "Forbidden: token not accepted by this server", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), identityKey{}, Identity{
			Subject:  res.Subject,
			ClientID: res.ClientID,
			Scope:    res.Scope,
			Username: res.Username,
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
// discover the authorization server. Field names match the MCP client's reader.
type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// MetadataHandler serves the protected-resource metadata at MetadataPath.
func (a *Authenticator) MetadataHandler() http.Handler {
	body, _ := json.Marshal(protectedResourceMetadata{
		Resource:               a.cfg.Resource,
		AuthorizationServers:   []string{a.cfg.Issuer},
		ScopesSupported:        []string{"openid"},
		BearerMethodsSupported: []string{"header"},
	})
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}

// challenge writes a 401 with a WWW-Authenticate header pointing at the
// protected-resource metadata, per RFC 9728 / the MCP authorization spec.
func (a *Authenticator) challenge(w http.ResponseWriter, detail string) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.metadataURL()+`"`)
	http.Error(w, "Unauthorized: "+detail, http.StatusUnauthorized)
}

// metadataURL is the absolute URL of the protected-resource metadata, derived
// from the resource's scheme+host so it sits at the host root.
func (a *Authenticator) metadataURL() string {
	res := a.cfg.Resource
	if i := strings.Index(res, "://"); i >= 0 {
		rest := res[i+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			return res[:i+3] + rest[:slash] + MetadataPath
		}
		return res + MetadataPath
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
