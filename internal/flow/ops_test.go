package flow

import (
	"testing"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

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
