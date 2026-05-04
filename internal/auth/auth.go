// Package auth provides proxy authentication middleware and context key utilities.
package auth

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/lm/polar-flow-mcp/internal/config"
)

// userIDKey is the unexported context key type for user identity (per D-12).
// Unexported struct type prevents cross-package key collisions.
type userIDKey struct{}

// UserIDKey is the exported singleton key instance for use with context.WithValue.
// Exported so that main.go's WithHTTPContextFunc can pass the identity to mcp-go.
//
//nolint:gochecknoglobals
var UserIDKey = userIDKey{}

// UserIDFromContext extracts the authenticated user identity from ctx.
// Returns ("", false) if not present — call sites MUST handle the missing case.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey{}).(string)
	return v, ok
}

// Middleware returns an http.Handler that enforces the proxy shared-secret contract.
//
// Security ordering (D-04, LOCKED — must not be inverted):
//  1. Compare PROXY_SHARED_SECRET header using subtle.ConstantTimeCompare → 403 on mismatch
//  2. Read identity header → 403 + WARN log if missing or empty
//  3. Inject identity into context → call next handler
//
// The secret check MUST be the first operation; reading the identity header
// before the secret check would allow header spoofing if the secret is wrong.
func Middleware(secret, identityHeader string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step 1: Shared-secret check FIRST (D-04).
		// subtle.ConstantTimeCompare prevents timing-oracle on the secret value.
		provided := r.Header.Get(config.ProxySecretHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			if provided != "" {
				// Non-empty wrong secret: probable spoofing attempt.
				slog.Warn("proxy secret mismatch — probable spoofing attempt",
					"remote_addr", r.RemoteAddr,
					"method", r.Method,
					"path", r.URL.Path,
				)
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Step 2: Identity header extraction (AFTER secret check — D-04).
		identity := r.Header.Get(identityHeader)
		if identity == "" {
			slog.Warn("missing identity header — probable proxy misconfiguration",
				"identity_header", identityHeader,
				"remote_addr", r.RemoteAddr,
				"path", r.URL.Path,
			)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Step 3: Inject identity into context.
		ctx := context.WithValue(r.Context(), userIDKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
