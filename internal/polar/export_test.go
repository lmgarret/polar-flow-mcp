package polar

// SetTokenEndpoint overrides tokenEndpoint for tests. Returns a restore function.
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
