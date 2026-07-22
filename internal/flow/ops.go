package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ogen-go/ogen/validate"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// wrapFlowError enriches ogen's UnexpectedStatusCodeError with the upstream
// response body, which ogen captures but omits from its Error() string.
func wrapFlowError(op string, err error) error {
	var sce *validate.UnexpectedStatusCodeError
	if errors.As(err, &sce) && sce.Payload != nil {
		body, _ := io.ReadAll(io.LimitReader(sce.Payload.Body, 512))
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			return fmt.Errorf("flow: %s: upstream %d: %s", op, sce.StatusCode, trimmed)
		}
		return fmt.Errorf("flow: %s: upstream %d (empty body)", op, sce.StatusCode)
	}
	return fmt.Errorf("flow: %s: %w", op, err)
}

// formatValidationError renders Polar's 400 body — a map of field name to a list
// of error codes, e.g. {"time": ["error.trainingTarget.twoTargetsForSameTime"]}
// — into an actionable one-line message naming the offending field(s). Without
// this the caller only sees a generic "validation rejected" and cannot tell what
// to fix.
func formatValidationError(v *gen.ValidationError) string {
	if v == nil || len(*v) == 0 {
		return "validation rejected (no detail in response body)"
	}
	parts := make([]string, 0, len(*v))
	nameFilterHit := false
	for field, codes := range *v {
		if field == nameContentFilterField {
			nameFilterHit = true
		}
		parts = append(parts, fmt.Sprintf("%s: %s", field, strings.Join(codes, ", ")))
	}
	sort.Strings(parts)
	msg := "validation rejected — " + strings.Join(parts, "; ")
	if nameFilterHit {
		// Polar runs the target name through a libinjection-style content filter
		// that rejects some innocuous free-text names (e.g. "5x(3min / 2min) - si
		// RAS") with this opaque generic message. The trigger is a whole-string,
		// prefix-sensitive fingerprint, so there is no reliable client-side
		// pre-filter — the fix is to reword or prefix the name and retry. Surface
		// that hint rather than leaving the caller with the untranslatable body.
		msg += " (hint: the workout name tripped Polar's server-side content " +
			"filter — reword it or add a leading prefix, e.g. \"Session - <name>\", and retry)"
	}
	return msg
}

// nameContentFilterField is the field key Polar returns when a training-target
// name is rejected by its server-side content filter (observed 2026-07-22 on
// both create and update). See internal/flow/openapi.yaml TrainingTargetCreate.name.
const nameContentFilterField = "trainingSessionTarget.name"

// UserInfo is the trimmed-down identity payload returned by GetUserInfo.
type UserInfo struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Country   string `json:"country,omitempty"`
}

// GetUserInfo wraps gen.GetCurrentUser and returns the identity fields most
// callers care about. Unauthorized is treated as a refresh failure (the
// transport should have already retried — surfacing here means refresh failed).
func (c *Client) GetUserInfo(ctx context.Context) (*UserInfo, error) {
	res, err := c.API.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("flow: get current user: %w", err)
	}
	switch v := res.(type) {
	case *gen.CurrentUserResponse:
		u := v.GetUser()
		out := &UserInfo{
			ID:    u.GetID(),
			Email: u.GetUserName(),
		}
		if f, ok := u.GetFirstName().Get(); ok {
			out.FirstName = f
		}
		if l, ok := u.GetLastName().Get(); ok {
			out.LastName = l
		}
		if c, ok := u.GetCountry().Get(); ok {
			out.Country = c
		}
		return out, nil
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get current user: unexpected response %T", res)
	}
}

// CreateTrainingTarget posts a structured target to /api/trainingtarget and
// returns the new target's numeric id (parsed from the response body).
// `body` must already be the ogen-shaped TrainingTargetCreate (the MCP layer
// owns the user-facing JSON → ogen mapping).
func (c *Client) CreateTrainingTarget(ctx context.Context, body *gen.TrainingTargetCreate) (int64, error) {
	res, err := c.API.CreateTrainingTarget(ctx, body, gen.CreateTrainingTargetParams{
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return 0, wrapFlowError("create training target", err)
	}
	switch v := res.(type) {
	case *gen.CreateTrainingTargetCreated:
		raw, rerr := io.ReadAll(io.LimitReader(v.Data, 1<<20))
		if rerr != nil {
			return 0, fmt.Errorf("flow: read create response: %w", rerr)
		}
		return extractTargetID(raw)
	case *gen.ValidationError:
		return 0, fmt.Errorf("flow: create training target: %s", formatValidationError(v))
	case *gen.Unauthorized:
		return 0, ErrLoginFailed
	default:
		return 0, fmt.Errorf("flow: create training target: unexpected response %T", res)
	}
}

// extractTargetID parses {"id": <number>} (or a bare numeric body) from the
// create-target response. The spec doesn't pin this format down; observed
// payloads have been either form.
func extractTargetID(body []byte) (int64, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return 0, fmt.Errorf("flow: create training target: empty response body")
	}
	// Object shape.
	var obj struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(body, &obj); err == nil && obj.ID != "" {
		id, perr := obj.ID.Int64()
		if perr == nil {
			return id, nil
		}
	}
	// Bare number.
	var bare json.Number
	if err := json.Unmarshal(body, &bare); err == nil {
		if id, perr := bare.Int64(); perr == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("flow: create training target: cannot parse id from %q", trimmed)
}

// GetTrainingTarget returns the server-normalized view of a target by id.
// Returns ErrTargetNotFound on 404.
func (c *Client) GetTrainingTarget(ctx context.Context, id int64) (*gen.GetTrainingTargetOK, error) {
	if id <= 0 {
		return nil, fmt.Errorf("flow: get training target: id must be > 0")
	}
	res, err := c.API.GetTrainingTarget(ctx, gen.GetTrainingTargetParams{ID: id})
	if err != nil {
		return nil, fmt.Errorf("flow: get training target: %w", err)
	}
	switch v := res.(type) {
	case *gen.GetTrainingTargetOK:
		return v, nil
	case *gen.GetTrainingTargetNotFound:
		return nil, ErrTargetNotFound
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get training target: unexpected response %T", res)
	}
}

// UpdateTrainingTarget replaces a target's body. The Polar Flow API is
// full-replace semantics: the supplied body completely overwrites the prior
// target. Returns ErrTargetNotFound on 404.
func (c *Client) UpdateTrainingTarget(ctx context.Context, id int64, body *gen.TrainingTargetCreate) error {
	if id <= 0 {
		return fmt.Errorf("flow: update training target: id must be > 0")
	}
	res, err := c.API.UpdateTrainingTarget(ctx, body, gen.UpdateTrainingTargetParams{
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
		ID:             id,
	})
	if err != nil {
		return fmt.Errorf("flow: update training target: %w", err)
	}
	switch v := res.(type) {
	case *gen.UpdateTrainingTargetOK:
		return nil
	case *gen.ValidationError:
		return fmt.Errorf("flow: update training target: %s", formatValidationError(v))
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: update training target: unexpected response %T", res)
	}
}

// GetCalendarWeekSummary returns one entry per ISO week intersecting [from, to].
// Polar caps the range at 45 days and rejects ISO-8601 dates — formatted internally
// as D.M.YYYY. Empty array when no sessions fall in the range.
//
// The per-item schema is not yet pinned in the spec (test accounts return `[]`),
// so callers get back the raw items as ogen sees them.
func (c *Client) GetCalendarWeekSummary(ctx context.Context, from, to time.Time) ([]gen.GetCalendarWeekSummaryOKItem, error) {
	if to.Sub(from) > 45*24*time.Hour {
		return nil, fmt.Errorf("flow: calendar week summary: range exceeds Polar's 45-day limit")
	}
	res, err := c.API.GetCalendarWeekSummary(ctx,
		&gen.GetCalendarWeekSummaryReq{From: convert.ToDotDMY(from), To: convert.ToDotDMY(to)},
		gen.GetCalendarWeekSummaryParams{XRequestedWith: gen.XRequestedWithXMLHttpRequest},
	)
	if err != nil {
		return nil, fmt.Errorf("flow: calendar week summary: %w", err)
	}
	switch v := res.(type) {
	case *gen.GetCalendarWeekSummaryOKApplicationJSON:
		return []gen.GetCalendarWeekSummaryOKItem(*v), nil
	case *gen.GetCalendarWeekSummaryBadRequest:
		return nil, fmt.Errorf("flow: calendar week summary: bad request (check date range)")
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: calendar week summary: unexpected response %T", res)
	}
}

// GetProgressViewSummary returns aggregated training totals (sessions, distance,
// duration, zone time, sport distribution, training-benefit distribution) for the
// supplied [from, to] range. `group` filters to a single sportId (use 0 for all
// sports). `timeFrame` is the bucket size for breakdowns: "6w", "3m", or "1y".
func (c *Client) GetProgressViewSummary(ctx context.Context, from, to time.Time, group, timeFrame string) (*gen.ProgressViewSummary, error) {
	// Progress endpoints want DD-MM-YYYY (dashes, zero-padded) — distinct from
	// the D.M.YYYY dot form the calendar endpoints use. Sending the dot form
	// here happened to work only on empty-range accounts.
	req := &gen.GetProgressViewSummaryReq{
		From: convert.ToDashDMY(from),
		To:   convert.ToDashDMY(to),
	}
	if group != "" {
		req.Group.SetTo(group)
	}
	if timeFrame != "" {
		req.TimeFrame.SetTo(timeFrame)
	}
	res, err := c.API.GetProgressViewSummary(ctx, req,
		gen.GetProgressViewSummaryParams{XRequestedWith: gen.XRequestedWithXMLHttpRequest},
	)
	if err != nil {
		var sce *validate.UnexpectedStatusCodeError
		if errors.As(err, &sce) && sce.StatusCode == http.StatusNotFound {
			// Account has no progress data in this range — return zeros per the tool contract.
			return &gen.ProgressViewSummary{}, nil
		}
		return nil, wrapFlowError("progress view summary", err)
	}
	switch v := res.(type) {
	case *gen.ProgressViewSummary:
		return v, nil
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: progress view summary: unexpected response %T", res)
	}
}

// DeleteTrainingTarget deletes the target with the given numeric id.
// Returns ErrTargetNotFound on 404.
func (c *Client) DeleteTrainingTarget(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("flow: delete training target: id must be > 0")
	}
	res, err := c.API.DeleteTrainingTarget(ctx, gen.DeleteTrainingTargetParams{ID: id})
	if err != nil {
		return fmt.Errorf("flow: delete training target: %w", err)
	}
	switch res.(type) {
	case *gen.DeleteTrainingTargetOK:
		return nil
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		// ogen surfaces 404 by returning an *ogenerrors.UnexpectedStatusCode
		// from the call itself, not via the Res sum-type. Map the type-default
		// case to ErrTargetNotFound is therefore not strictly correct — but the
		// generated DeleteTrainingTargetRes set is {OK, Unauthorized} only, so
		// anything else here means a genuine surprise.
		return fmt.Errorf("flow: delete training target: unexpected response %T", res)
	}
}

// CalendarTarget is a trimmed view of a CalendarEvent filtered to training-target
// entries.
type CalendarTarget struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Start string `json:"start"`        // ISO 8601 UTC
	URL   string `json:"url,omitempty"`
}

// ListTrainingTargets returns calendar events whose `type` is TRAININGTARGET,
// in the date range [from, to] (inclusive, D.M.YYYY format per the API).
func (c *Client) ListTrainingTargets(ctx context.Context, from, to time.Time) ([]CalendarTarget, error) {
	events, err := c.GetCalendarEvents(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]CalendarTarget, 0, len(events))
	for _, e := range events {
		if t, ok := e.GetType().Get(); !ok || !strings.EqualFold(t, "TRAININGTARGET") {
			continue
		}
		var item CalendarTarget
		if id, ok := e.GetListItemId().Get(); ok {
			item.ID = int64(id)
		}
		if v, ok := e.GetTitle().Get(); ok {
			item.Title = v
		}
		// start is free-form (some event kinds send a numeric epoch); for
		// TRAININGTARGET events it is always an ISO 8601 string.
		var start string
		if err := json.Unmarshal(e.GetStart(), &start); err == nil {
			item.Start = start
		}
		if v, ok := e.GetURL().Get(); ok {
			item.URL = v
		}
		out = append(out, item)
	}
	return out, nil
}

// GetCalendarEvents returns the raw list of calendar events in [from, to].
// Dates are converted to the D.M.YYYY format expected by the upstream API.
func (c *Client) GetCalendarEvents(ctx context.Context, from, to time.Time) ([]gen.CalendarEvent, error) {
	res, err := c.API.GetCalendarEvents(ctx, gen.GetCalendarEventsParams{
		Start: convert.ToDotDMY(from),
		End:   convert.ToDotDMY(to),
	})
	if err != nil {
		return nil, fmt.Errorf("flow: get calendar events: %w", err)
	}
	switch v := res.(type) {
	case *gen.GetCalendarEventsOKApplicationJSON:
		return []gen.CalendarEvent(*v), nil
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get calendar events: unexpected response %T", res)
	}
}

// ListTrainingSessions returns completed training sessions in [from, to] for
// the given userID. userID must be obtained from GetUserInfo first.
func (c *Client) ListTrainingSessions(ctx context.Context, userID int64, from, to time.Time) ([]gen.TrainingSessionSummary, error) {
	req := &gen.ListTrainingSessionsReq{
		UserId:   int(userID),
		FromDate: from.Format("2006-01-02"),
		ToDate:   to.Format("2006-01-02"),
	}
	res, err := c.API.ListTrainingSessions(ctx, req, gen.ListTrainingSessionsParams{
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return nil, wrapFlowError("list training sessions", err)
	}
	switch v := res.(type) {
	case *gen.ListTrainingSessionsOKApplicationJSON:
		return []gen.TrainingSessionSummary(*v), nil
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: list training sessions: unexpected response %T", res)
	}
}

// GetTrainingSessionSummary returns the summary view of a completed session.
func (c *Client) GetTrainingSessionSummary(ctx context.Context, id int64) (*gen.SessionSummary, error) {
	res, err := c.API.GetTrainingSessionSummary(ctx, gen.GetTrainingSessionSummaryParams{ID: id})
	if err != nil {
		return nil, fmt.Errorf("flow: get session summary: %w", err)
	}
	switch v := res.(type) {
	case *gen.SessionSummary:
		return v, nil
	case *gen.GetTrainingSessionSummaryNotFound:
		return nil, ErrTargetNotFound
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get session summary: unexpected response %T", res)
	}
}

// GetTrainingSessionDetails returns the lap/sample-level details for a session.
func (c *Client) GetTrainingSessionDetails(ctx context.Context, id int64) (*gen.SessionDetails, error) {
	res, err := c.API.GetTrainingSessionDetails(ctx, gen.GetTrainingSessionDetailsParams{ID: id})
	if err != nil {
		return nil, fmt.Errorf("flow: get session details: %w", err)
	}
	switch v := res.(type) {
	case *gen.SessionDetails:
		return v, nil
	case *gen.GetTrainingSessionDetailsNotFound:
		return nil, ErrTargetNotFound
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get session details: unexpected response %T", res)
	}
}

// CreateTrainingSession posts a manual session entry to /api/training/create —
// the endpoint behind the "Manual training result" form. The response body is
// empty (Polar does not return the new session id); call ListTrainingSessions
// afterwards if you need to look it up.
//
// WARNING: this writes a real session into Polar Flow. It contributes to
// weekly volume, progress summaries, and training-load models. Intended for
// user-initiated logging of off-watch sessions, not for synthesizing data.
func (c *Client) CreateTrainingSession(ctx context.Context, body *gen.TrainingSessionCreate) error {
	res, err := c.API.CreateTrainingSession(ctx, body, gen.CreateTrainingSessionParams{
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return fmt.Errorf("flow: create training session: %w", err)
	}
	switch res.(type) {
	case *gen.CreateTrainingSessionOK:
		return nil
	case *gen.CreateTrainingSessionBadRequest:
		return fmt.Errorf("flow: create training session: validation rejected")
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: create training session: unexpected response %T", res)
	}
}

// ListSports returns the Polar sport catalogue as a map of numeric sport id
// (string key) to the internal sport-name constant (e.g. "1" → "RUNNING").
// These ids are the sport_id values used by training targets and sessions.
func (c *Client) ListSports(ctx context.Context) (gen.SportsMap, error) {
	res, err := c.API.GetSports(ctx)
	if err != nil {
		return nil, fmt.Errorf("flow: get sports: %w", err)
	}
	switch v := res.(type) {
	case *gen.SportsMap:
		return *v, nil
	case *gen.GetSportsNotFound:
		return nil, fmt.Errorf("flow: get sports: not found")
	case *gen.GetSportsInternalServerError:
		return nil, fmt.Errorf("flow: get sports: server error")
	default:
		return nil, fmt.Errorf("flow: get sports: unexpected response %T", res)
	}
}
