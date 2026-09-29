package flow

import (
	"context"
	"errors"
	"fmt"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// CheckTrainingSession reports whether a completed session exists and belongs
// to this account, via the summary endpoint: 200 → the summary, 404 →
// ErrSessionNotFound, 403 → ErrNotOwned.
//
// It is the existence check the session writes need, because
// DELETE /api/training/deleteTrainingSession/{id}/ answers 200 for an id that
// does not exist — a delete alone cannot tell "deleted" from "never there".
func (c *Client) CheckTrainingSession(ctx context.Context, id int64) (*gen.SessionSummary, error) {
	if id <= 0 {
		return nil, fmt.Errorf("flow: session id must be > 0")
	}
	res, err := c.API.GetTrainingSessionSummary(ctx, gen.GetTrainingSessionSummaryParams{ID: id})
	if err != nil {
		return nil, wrapFlowError("get session summary", err)
	}
	switch v := res.(type) {
	case *gen.SessionSummary:
		return v, nil
	case *gen.GetTrainingSessionSummaryNotFound:
		return nil, ErrSessionNotFound
	case *gen.GetTrainingSessionSummaryForbidden:
		return nil, ErrNotOwned
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get session summary: unexpected response %T", res)
	}
}

// DeleteTrainingSession deletes a completed session. The caller should check
// existence first (CheckTrainingSession): Flow returns 200 for unknown ids.
// A 404 here means the session belongs to another account.
func (c *Client) DeleteTrainingSession(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("flow: session id must be > 0")
	}
	res, err := c.API.DeleteTrainingSession(ctx, gen.DeleteTrainingSessionParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("delete training session", err)
	}
	switch res.(type) {
	case *gen.DeleteTrainingSessionOK:
		return nil
	case *gen.DeleteTrainingSessionNotFound:
		return ErrNotOwned
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: delete training session: unexpected response %T", res)
	}
}

// EditTrainingSession sends the full "edit session" form. Flow answers every
// out-of-range value with a bare 500 and no body, so callers must validate
// first; the 500 is surfaced with that explanation rather than as a generic
// server error.
func (c *Client) EditTrainingSession(ctx context.Context, id int64, body *gen.TrainingSessionEdit) error {
	if id <= 0 {
		return fmt.Errorf("flow: session id must be > 0")
	}
	res, err := c.API.EditTrainingSession(ctx, body, gen.EditTrainingSessionParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("edit training session", err)
	}
	switch res.(type) {
	case *gen.EditTrainingSessionOK:
		return nil
	case *gen.EditTrainingSessionNotFound:
		return ErrSessionNotFound
	case *gen.EditTrainingSessionForbidden:
		return ErrNotOwned
	case *gen.EditTrainingSessionInternalServerError:
		return errors.New("flow: edit training session: Polar rejected a field value " +
			"(it answers any out-of-range or unknown value with a bare 500 and does not say which)")
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: edit training session: unexpected response %T", res)
	}
}

// UpdateTrainingSessionData updates only a session's note and/or feeling —
// the partial endpoint behind the session page's inline note box. Unlike
// EditTrainingSession it never touches the other fields.
func (c *Client) UpdateTrainingSessionData(ctx context.Context, id int64, body *gen.TrainingSessionDataUpdate) error {
	if id <= 0 {
		return fmt.Errorf("flow: session id must be > 0")
	}
	res, err := c.API.UpdateTrainingSessionData(ctx, body, gen.UpdateTrainingSessionDataParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("update training session note/feeling", err)
	}
	switch res.(type) {
	case *gen.UpdateTrainingSessionDataOK:
		return nil
	case *gen.UpdateTrainingSessionDataNotFound:
		return ErrSessionNotFound
	case *gen.UpdateTrainingSessionDataBadRequest:
		return errors.New("flow: update training session note/feeling: rejected " +
			"(note over 10000 characters, feeling out of range, or another account's session)")
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: update training session note/feeling: unexpected response %T", res)
	}
}
