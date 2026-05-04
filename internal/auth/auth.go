// Package auth provides proxy authentication middleware and context key utilities.
package auth

import (
	"context"
	"net/http"
)

// userIDKey is the unexported context key type for user identity.
// Using an unexported struct type prevents cross-package key collisions.
type userIDKey struct{}

// UserIDKey is the singleton key instance used with context.WithValue.
var UserIDKey = userIDKey{} //nolint:gochecknoglobals

// UserIDFromContext extracts the user identity string from the context.
// Returns ("", false) if the identity is not present, forcing call sites to handle the missing case.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey{}).(string)
	return v, ok
}

// Middleware returns an http.Handler that enforces proxy authentication.
// Stub: passes through all requests. Real logic added in Plan 04.
func Middleware(secret, identityHeader string, next http.Handler) http.Handler {
	return next
}
