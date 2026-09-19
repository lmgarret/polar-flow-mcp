package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
)

// APIKeyGuard requires a fixed, pre-shared API key in the Authorization: Bearer
// header on every request. It is the lightweight alternative to the full OAuth
// 2.1 flow: no issuer, no signing key, no forward-auth proxy, no browser — just
// one secret configured on the server and pasted into the client.
//
// Trade-offs versus OAuth: the secret is long-lived and shared by every caller,
// there is no per-client identity, and revocation means restarting with a new
// key. It is suitable for a private deployment behind TLS where the operator is
// the only caller; use OAuth when several people (or Claude.ai's connector UI)
// must connect.
type APIKeyGuard struct {
	// digest is the SHA-256 of the configured key. Comparing digests rather
	// than the raw strings keeps the comparison constant-time regardless of
	// the presented token's length.
	digest [sha256.Size]byte
	log    *slog.Logger
}

// NewAPIKeyGuard builds a guard for the given key. The caller is responsible
// for rejecting an empty or too-short key (config.Load does this at startup).
func NewAPIKeyGuard(key string, log *slog.Logger) *APIKeyGuard {
	if log == nil {
		log = slog.Default()
	}
	return &APIKeyGuard{digest: sha256.Sum256([]byte(key)), log: log}
}

// Middleware wraps a handler, requiring the configured API key as a Bearer
// token. It is fail-closed: a missing or mismatched key is a 401.
func (g *APIKeyGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			g.challenge(w, "missing bearer token")
			return
		}
		got := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(got[:], g.digest[:]) != 1 {
			g.log.Warn("api key rejected", "remote", r.RemoteAddr, "path", r.URL.Path)
			g.challenge(w, "invalid api key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// challenge writes a 401 with a bare Bearer challenge. Unlike the OAuth path
// there is no resource metadata to point at — there is nothing for a client to
// discover or negotiate, the key is configured out of band.
func (g *APIKeyGuard) challenge(w http.ResponseWriter, detail string) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	http.Error(w, "Unauthorized: "+detail, http.StatusUnauthorized)
}
