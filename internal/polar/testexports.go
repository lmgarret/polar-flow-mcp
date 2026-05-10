//go:build polartest

package polar

// SetTokenEndpoint overrides tokenEndpoint for tests. Returns a restore function.
// Gated behind the `polartest` build tag so it does NOT ship in production binaries (CR-03).
// All `go test` invocations in this repo pass `-tags=polartest` (see Makefile and CI).
func SetTokenEndpoint(s string) func() {
	orig := tokenEndpoint
	tokenEndpoint = s
	return func() { tokenEndpoint = orig }
}

// SetRegisterEndpoint overrides registerEndpoint for tests. Returns a restore function.
// Gated behind the `polartest` build tag (see SetTokenEndpoint).
func SetRegisterEndpoint(s string) func() {
	orig := registerEndpoint
	registerEndpoint = s
	return func() { registerEndpoint = orig }
}
