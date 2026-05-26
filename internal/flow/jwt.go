// Package flow wraps the generated Polar Flow web API client with the
// cookie-based session management documented in
// polar-openapi-maker/docs/auth.md.
package flow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// playSessionData captures the nested `data` object inside the PLAY_SESSION_FLOW
// JWT. Only csrfToken is load-bearing for our refresh path.
type playSessionData struct {
	CsrfToken string `json:"csrfToken"`
}

type playSessionPayload struct {
	Data playSessionData `json:"data"`
}

// extractPlayCsrfToken reads PLAY_SESSION_FLOW.data.csrfToken — the value
// needed as the ?csrfToken= query param on /flowSso/login.
func extractPlayCsrfToken(jwt string) (string, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("flow: PLAY_SESSION_FLOW not a JWT (got %d segments)", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("flow: decode PLAY_SESSION_FLOW payload: %w", err)
	}
	var p playSessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("flow: unmarshal PLAY_SESSION_FLOW payload: %w", err)
	}
	if p.Data.CsrfToken == "" {
		return "", fmt.Errorf("flow: PLAY_SESSION_FLOW has empty data.csrfToken")
	}
	return p.Data.CsrfToken, nil
}
