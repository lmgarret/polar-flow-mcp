package flow

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// ErrNotLinked is returned when no credentials are available (neither a usable
// cookie jar nor email/password). Callers translate this into MCP user-facing
// guidance ("set POLAR_EMAIL/POLAR_PASSWORD in env").
var ErrNotLinked = errors.New("flow: no Polar credentials configured")

// ErrLoginFailed is returned when the login chain refuses the supplied
// credentials. Distinct from ErrNotLinked so callers can show "credentials
// rejected" vs "credentials missing".
var ErrLoginFailed = errors.New("flow: Polar credentials rejected")

// ErrTargetNotFound mirrors the previous internal/polar error so call sites
// can distinguish 404 from other failures via errors.Is.
var ErrTargetNotFound = errors.New("flow: training target not found")

// isNotAuthenticatedBody returns true if the body matches the Polar Flow 401
// shape: {"error":"NotAuthenticated","redirect":"/"} (docs/auth.md).
func isNotAuthenticatedBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if !bytes.Contains(body, []byte("NotAuthenticated")) {
		return false
	}
	var probe struct {
		Error string `json:"error"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&probe); err != nil {
		return false
	}
	return strings.EqualFold(probe.Error, "NotAuthenticated")
}
