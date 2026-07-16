package flow

import (
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
