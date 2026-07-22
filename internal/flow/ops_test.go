package flow

import (
	"strings"
	"testing"

	"github.com/go-faster/jx"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// TestCalendarEventDecodeNullDescription guards the regression where the
// getCalendarEvents response failed to decode with:
//
//	decode CalendarEvent: decode field "description": unexpected byte 110 'n'
//
// EXERCISE (recorded-session) events send `description: null`, so the field
// must be nullable — decoding a null must succeed and read back as a null
// OptNilString, not error.
func TestCalendarEventDecodeNullDescription(t *testing.T) {
	const body = `{"type":"EXERCISE","title":"Morning Run","description":null}`

	var ev gen.CalendarEvent
	if err := ev.Decode(jx.DecodeStr(body)); err != nil {
		t.Fatalf("decode CalendarEvent with null description: %v", err)
	}

	d := ev.GetDescription()
	if !d.Set {
		t.Fatalf("Description.Set = false, want true (field was present)")
	}
	if !d.Null {
		t.Fatalf("Description.Null = false, want true (value was null)")
	}
}

func TestFormatValidationError(t *testing.T) {
	tests := []struct {
		name string
		in   *gen.ValidationError
		want string
	}{
		{
			name: "nil",
			in:   nil,
			want: "validation rejected (no detail in response body)",
		},
		{
			name: "empty",
			in:   &gen.ValidationError{},
			want: "validation rejected (no detail in response body)",
		},
		{
			name: "single field",
			in:   &gen.ValidationError{"time": {"error.trainingTarget.twoTargetsForSameTime"}},
			want: "validation rejected — time: error.trainingTarget.twoTargetsForSameTime",
		},
		{
			name: "multiple fields sorted",
			in:   &gen.ValidationError{"time": {"error.b"}, "name": {"error.a", "error.c"}},
			want: "validation rejected — name: error.a, error.c; time: error.b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatValidationError(tt.in); got != tt.want {
				t.Fatalf("formatValidationError() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatValidationErrorNameContentFilterHint guards that a rejection on the
// trainingSessionTarget.name field (Polar's server-side content filter) is
// annotated with an actionable reword/prefix hint — the raw localized body is
// otherwise opaque about how to recover. See the name-filter note in
// internal/flow/openapi.yaml (TrainingTargetCreate.name).
func TestFormatValidationErrorNameContentFilterHint(t *testing.T) {
	in := &gen.ValidationError{
		nameContentFilterField: {"Un problème inattendu est survenu. Réessayez."},
	}
	got := formatValidationError(in)
	if !strings.Contains(got, nameContentFilterField) {
		t.Fatalf("expected message to name the field, got %q", got)
	}
	if !strings.Contains(got, "content filter") || !strings.Contains(got, "prefix") {
		t.Fatalf("expected reword/prefix hint, got %q", got)
	}
}
