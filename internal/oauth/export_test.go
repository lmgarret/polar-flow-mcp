package oauth

// SetAuthorizeURL overrides the Polar authorization page URL for tests. Returns a restore function.
func SetAuthorizeURL(s string) func() {
	orig := authorizeURL
	authorizeURL = s
	return func() { authorizeURL = orig }
}
