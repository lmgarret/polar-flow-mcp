package polar

// SetTokenEndpoint overrides tokenEndpoint for tests. Returns a restore function.
// This function is in a non-test file so it is accessible from test binaries of other
// packages (e.g., internal/oauth). It is safe to expose because polar is an internal package.
func SetTokenEndpoint(s string) func() {
	orig := tokenEndpoint
	tokenEndpoint = s
	return func() { tokenEndpoint = orig }
}

// SetRegisterEndpoint overrides registerEndpoint for tests. Returns a restore function.
func SetRegisterEndpoint(s string) func() {
	orig := registerEndpoint
	registerEndpoint = s
	return func() { registerEndpoint = orig }
}
