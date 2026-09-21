package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// User confirmation for the two tools that touch real diary data:
// create_training_session (writes a session that counts toward Polar's
// training-load model) and delete_training_target (irreversible). Both used to
// rely on prose in the tool description to keep a coach model from firing them
// unprompted — a hint, not a gate. Multi round-trip requests (SEP-2322, new in
// mcp-go v1.1.0) make it a real one: the handler answers "I need confirmation
// first", the client puts the question to the user, then retries the same call
// with the answer attached. One handler serves both protocol eras — against a
// client older than 2026-07-28, mcp-go bridges the exchange by issuing the
// server-initiated elicitation/create that client understands.
//
// The footgun that shapes the code below: that legacy bridge does not check
// whether the client can answer. stdioSession and streamableHttpSession both
// implement server.SessionWithElicitation unconditionally, so the bridge will
// send elicitation/create to a client that never declared the capability and
// then fail the entire tool call with ErrElicitationNotSupported (or block
// until the request times out). So we probe the client ourselves and skip the
// gate when it cannot answer: an unconfirmable call behaves exactly as it did
// before the gate existed, rather than breaking on hosts without elicitation
// support.

// confirmID keys the confirmation within one tool call. There is never more
// than one input request per call, so a single constant is enough.
const confirmID = "confirm"

// confirmSchema is the requested schema for a plain yes/no confirmation. Form
// mode requires a schema, but what we act on is the elicitation *action*
// (accept / decline / cancel), not a submitted field — so the object is
// deliberately empty and the client renders a bare confirm/cancel prompt.
var confirmSchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{},
}

// requireConfirm gates a handler behind an explicit user confirmation.
//
// proceed is true when the handler should carry on: the user accepted, or the
// client cannot be asked. Otherwise the returned result is final and the
// handler must return it unchanged — either the input request carrying the
// question, or the cancellation notice.
//
// message is a closure because it is only needed on the leg that actually
// asks; building it may cost an API round-trip (see describeTarget).
func requireConfirm(
	ctx context.Context,
	req mcpgo.CallToolRequest,
	tool string,
	message func() string,
) (*mcpgo.CallToolResult, bool) {
	// Retry leg: the client has been round the loop and carries the answer.
	if answer := server.ElicitationResponse(req.Params.InputResponses, confirmID); answer != nil {
		if answer.Action == mcpgo.ElicitationResponseActionAccept {
			return nil, true
		}
		slog.Info("tool: not confirmed", "tool", tool, "action", answer.Action)
		return mcpgo.NewToolResultText(
			"Cancelled: the user did not confirm. Nothing was created, changed, or deleted. " +
				"Do not retry unless the user asks again.",
		), false
	}
	if !canElicit(ctx) {
		return nil, true
	}
	// First leg: ask. The client retries the call with the original arguments,
	// so there is no handler state to carry; the request state is the tool name
	// purely so the exchange is identifiable in a transcript.
	return server.NewInputRequestBuilder(tool).
		Elicit(confirmID, mcpgo.ElicitationParams{
			Mode:            mcpgo.ElicitationModeForm,
			Message:         message(),
			RequestedSchema: confirmSchema,
		}).
		ToolResult(), false
}

// canElicit reports whether the connected client can actually answer an
// elicitation request. See the package-level note above for why this cannot be
// left to mcp-go.
func canElicit(ctx context.Context) bool {
	if server.IsModernRequest(ctx) {
		// 2026-07-28+: capabilities ride in every request's _meta, and the
		// server rejects a modern request that omits them — so a nil here means
		// we genuinely do not know, and not knowing is a no.
		info := server.RequestProtocolInfoFromContext(ctx)
		return info != nil &&
			info.ClientCapabilities != nil &&
			info.ClientCapabilities.Elicitation != nil
	}
	// Legacy: the session type implements SessionWithElicitation whether or not
	// the client can handle the request, so what the client declared at
	// initialize is the only reliable signal.
	sess, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo)
	return ok && sess.GetClientCapabilities().Elicitation != nil
}

// describeTarget renders a target as something a user can actually agree to
// delete. "Delete target 7286431?" is not an answerable question; "Delete
// \"5x1km Threshold\" (2026-06-02T09:00)?" is. Falls back to the bare id when
// the read fails — the prompt is still worth showing without the name, and the
// delete itself reports a missing target on its own.
func describeTarget(ctx context.Context, fc *flow.Client, id int64) string {
	t, err := fc.GetTrainingTarget(ctx, id)
	if err != nil || strings.TrimSpace(t.Name) == "" {
		return fmt.Sprintf("the training target with id %d", id)
	}
	if t.Datetime != "" {
		return fmt.Sprintf("the training target %q scheduled for %s (id %d)", t.Name, t.Datetime, id)
	}
	return fmt.Sprintf("the training target %q (id %d)", t.Name, id)
}
