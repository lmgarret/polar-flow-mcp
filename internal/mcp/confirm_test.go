package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// fakeSession is a server.SessionWithClientInfo that reports exactly the
// capabilities a test hands it. The real stdio and streamable-HTTP sessions
// implement SessionWithElicitation whether or not the client can answer one,
// which is the whole reason canElicit reads the declared capabilities instead.
type fakeSession struct {
	caps mcpgo.ClientCapabilities
}

func (f *fakeSession) Initialize()                                           {}
func (f *fakeSession) Initialized() bool                                     { return true }
func (f *fakeSession) NotificationChannel() chan<- mcpgo.JSONRPCNotification { return nil }
func (f *fakeSession) SessionID() string                                     { return "test-session" }
func (f *fakeSession) GetClientInfo() mcpgo.Implementation                   { return mcpgo.Implementation{} }
func (f *fakeSession) SetClientInfo(mcpgo.Implementation)                    {}
func (f *fakeSession) GetClientCapabilities() mcpgo.ClientCapabilities       { return f.caps }
func (f *fakeSession) SetClientCapabilities(c mcpgo.ClientCapabilities)      { f.caps = c }

var _ server.SessionWithClientInfo = (*fakeSession)(nil)

// legacyCtx builds a context for a pre-2026-07-28 request, where the capability
// the client declared at initialize is the only signal available.
func legacyCtx(elicitation bool) context.Context {
	sess := &fakeSession{}
	if elicitation {
		sess.caps.Elicitation = &mcpgo.ElicitationCapability{}
	}
	return (&server.MCPServer{}).WithContext(context.Background(), sess)
}

// modernCtx builds a context for a 2026-07-28+ request, whose capabilities ride
// in the request's own _meta rather than on the session.
func modernCtx(elicitation bool) context.Context {
	caps := &mcpgo.ClientCapabilities{}
	if elicitation {
		caps.Elicitation = &mcpgo.ElicitationCapability{}
	}
	return server.WithRequestProtocolInfo(context.Background(), &server.RequestProtocolInfo{
		Modern:             true,
		ProtocolVersion:    "2026-07-28",
		ClientCapabilities: caps,
	})
}

func TestCanElicit(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"legacy client declaring elicitation", legacyCtx(true), true},
		{"legacy client without elicitation", legacyCtx(false), false},
		{"modern client declaring elicitation", modernCtx(true), true},
		{"modern client without elicitation", modernCtx(false), false},
		// No session and no protocol info at all: not knowing is a no, so the
		// gate is skipped rather than the call failing.
		{"bare context", context.Background(), false},
	}
	for _, tt := range tests {
		if got := canElicit(tt.ctx); got != tt.want {
			t.Errorf("canElicit(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func msg() string { return "Delete the thing?" }

// A client that cannot answer must not be asked: the handler proceeds exactly
// as it did before the gate existed. Asking anyway is what would fail the whole
// tool call with ErrElicitationNotSupported.
func TestRequireConfirm_SkippedWhenClientCannotAnswer(t *testing.T) {
	result, proceed := requireConfirm(legacyCtx(false), req(nil), "delete_training_target", msg)
	if !proceed {
		t.Fatalf("proceed = false, want true for a client without elicitation")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
	}
}

// First leg: the handler stops and asks.
func TestRequireConfirm_AsksOnFirstCall(t *testing.T) {
	result, proceed := requireConfirm(legacyCtx(true), req(nil), "delete_training_target", msg)
	if proceed {
		t.Fatalf("proceed = true, want false — the handler must stop and ask")
	}
	if result == nil {
		t.Fatal("result = nil, want an input-required result")
	}
	if !result.NeedsInput() {
		t.Errorf("NeedsInput() = false, want true (ResultType = %q)", result.ResultType)
	}
	request, ok := result.InputRequests[confirmID]
	if !ok {
		t.Fatalf("no input request under %q, got keys %v", confirmID, keysOf(result.InputRequests))
	}
	if request.Method != mcpgo.MethodElicitationCreate {
		t.Errorf("method = %q, want %q", request.Method, mcpgo.MethodElicitationCreate)
	}
	if request.Elicitation == nil {
		t.Fatal("elicitation params are nil")
	}
	if request.Elicitation.Message != msg() {
		t.Errorf("message = %q, want %q", request.Elicitation.Message, msg())
	}
	// Form mode is rejected by mcp-go's own validation without a schema, so the
	// empty-object schema is load-bearing, not decoration.
	if err := request.Elicitation.Validate(); err != nil {
		t.Errorf("elicitation params do not validate: %v", err)
	}
}

// Retry leg: accept lets the handler through, decline and cancel stop it.
func TestRequireConfirm_HonoursTheAnswer(t *testing.T) {
	tests := []struct {
		action      mcpgo.ElicitationResponseAction
		wantProceed bool
	}{
		{mcpgo.ElicitationResponseActionAccept, true},
		{mcpgo.ElicitationResponseActionDecline, false},
		{mcpgo.ElicitationResponseActionCancel, false},
	}
	for _, tt := range tests {
		r := req(nil)
		r.Params.InputResponses = mcpgo.InputResponses{
			confirmID: mcpgo.NewElicitationInputResponse(mcpgo.ElicitationResult{ElicitationResponse: mcpgo.ElicitationResponse{Action: tt.action}}),
		}
		result, proceed := requireConfirm(legacyCtx(true), r, "delete_training_target", msg)
		if proceed != tt.wantProceed {
			t.Errorf("action %q: proceed = %v, want %v", tt.action, proceed, tt.wantProceed)
		}
		if tt.wantProceed {
			continue
		}
		if result == nil {
			t.Errorf("action %q: result = nil, want a cancellation notice", tt.action)
			continue
		}
		// The cancellation is a normal result, not a tool error: nothing went
		// wrong, the user simply said no.
		if result.IsError {
			t.Errorf("action %q: IsError = true, want a plain text result", tt.action)
		}
		if result.NeedsInput() {
			t.Errorf("action %q: asked again instead of giving up", tt.action)
		}
	}
}

func keysOf(m mcpgo.InputRequests) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
