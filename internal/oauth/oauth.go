// Package oauth provides HTTP handlers for the Polar OAuth2 authorization flow.
package oauth

import "net/http"

// LoginHandler initiates the Polar OAuth2 authorization flow.
// Stub: real implementation in Phase 2.
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// CallbackHandler handles the Polar OAuth2 callback.
// Stub: real implementation in Phase 2.
func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
