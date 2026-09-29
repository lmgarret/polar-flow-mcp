package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
)

// Strict argument accessors for the write tools added alongside favorites,
// routes and session edits. The older handlers read numbers through GetFloat,
// which turns a wrong type (a string id, a boolean) into the default and so
// into a confusing downstream error. These accessors reject the wrong type,
// non-integers where an integer is required, and out-of-range values up front,
// with a message naming the argument — before any request goes out.

// rawArg returns the argument value and whether it was supplied (a JSON null
// counts as not supplied).
func rawArg(req mcpgo.CallToolRequest, name string) (any, bool) {
	v, ok := req.GetArguments()[name]
	if !ok || v == nil {
		return nil, false
	}
	return v, true
}

// numberArg reads an optional number. present=false when absent or null.
func numberArg(req mcpgo.CallToolRequest, name string) (v float64, present bool, err error) {
	raw, ok := rawArg(req, name)
	if !ok {
		return 0, false, nil
	}
	switch n := raw.(type) {
	case float64:
		v = n
	case int:
		v = float64(n)
	case int64:
		v = float64(n)
	case json.Number:
		f, perr := n.Float64()
		if perr != nil {
			return 0, true, fmt.Errorf("%s must be a number, got %q", name, n.String())
		}
		v = f
	default:
		return 0, true, fmt.Errorf("%s must be a number, got %s", name, jsonTypeName(raw))
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, true, fmt.Errorf("%s must be a finite number", name)
	}
	return v, true, nil
}

// intArgRange reads an optional integer in [lo, hi]. unit is appended to the
// range in the error message ("seconds", "bpm", …).
func intArgRange(req mcpgo.CallToolRequest, name string, lo, hi int64, unit string) (v int64, present bool, err error) {
	f, present, err := numberArg(req, name)
	if err != nil || !present {
		return 0, present, err
	}
	if f != math.Trunc(f) {
		return 0, true, fmt.Errorf("%s must be a whole number, got %v", name, f)
	}
	if f < float64(lo) || f > float64(hi) {
		return 0, true, fmt.Errorf("%s must be between %d and %d%s, got %v", name, lo, hi, unitSuffix(unit), f)
	}
	return int64(f), true, nil
}

// floatArgRange reads an optional number in [lo, hi].
func floatArgRange(req mcpgo.CallToolRequest, name string, lo, hi float64, unit string) (v float64, present bool, err error) {
	f, present, err := numberArg(req, name)
	if err != nil || !present {
		return 0, present, err
	}
	if f < lo || f > hi {
		return 0, true, fmt.Errorf("%s must be between %v and %v%s, got %v", name, lo, hi, unitSuffix(unit), f)
	}
	return f, true, nil
}

// idArg reads a required positive integer id.
func idArg(req mcpgo.CallToolRequest, name string) (int64, error) {
	v, present, err := intArgRange(req, name, 1, math.MaxInt64>>11, "")
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive integer id: %w", name, err)
	}
	if !present {
		return 0, fmt.Errorf("%s is required (a positive integer id)", name)
	}
	return v, nil
}

// stringArg reads an optional string. present=false when absent or null.
func stringArg(req mcpgo.CallToolRequest, name string) (string, bool, error) {
	raw, ok := rawArg(req, name)
	if !ok {
		return "", false, nil
	}
	s, isStr := raw.(string)
	if !isStr {
		return "", true, fmt.Errorf("%s must be a string, got %s", name, jsonTypeName(raw))
	}
	return s, true, nil
}

// textArg reads an optional string of at most maxRunes characters, counted
// the way Polar counts them (convert.PolarTextLen: an emoji is 2). When
// nonBlank is set, a supplied value must contain a non-whitespace character.
func textArg(req mcpgo.CallToolRequest, name string, maxRunes int, nonBlank bool) (string, bool, error) {
	s, present, err := stringArg(req, name)
	if err != nil || !present {
		return s, present, err
	}
	if nonBlank && strings.TrimSpace(s) == "" {
		return "", true, fmt.Errorf("%s must not be empty or whitespace", name)
	}
	if n := convert.PolarTextLen(s); n > maxRunes {
		return "", true, fmt.Errorf("%s is %d characters; Polar's limit is %d", name, n, maxRunes)
	}
	return s, true, nil
}

func unitSuffix(unit string) string {
	if unit == "" {
		return ""
	}
	return " " + unit
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	default:
		return fmt.Sprintf("%T", v)
	}
}
