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

// ErrSessionNotFound is returned when a completed training session id does not
// exist (the summary endpoint answers 404).
var ErrSessionNotFound = errors.New("flow: training session not found")

// ErrFavoriteNotFound is returned when a favorite (or a favorite's exercise
// target) does not exist for this account.
var ErrFavoriteNotFound = errors.New("flow: favorite not found")

// ErrNotOwned is returned when the id exists but belongs to another Polar
// account (Flow answers 403 on reads, and 400/403/404 on writes depending on
// the endpoint).
var ErrNotOwned = errors.New("flow: that id belongs to another Polar account")

// ErrTargetTimeClash is returned when a training target would land on the
// same minute as another one: Flow refuses two targets at the same datetime
// (400 text/plain {"time":["error.trainingTarget.twoTargetsForSameTime"]}).
// The occupant may be invisible on the calendar — a create rejected for a
// phase name still stores a half-created target (see openapi.yaml).
var ErrTargetTimeClash = errors.New("flow: another training target is already scheduled at that exact time")

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
