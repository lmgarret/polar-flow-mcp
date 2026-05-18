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

// SetTrainingTargetsV4URL overrides trainingTargetsV4URL for tests. Returns a restore function.
// Gated behind the `polartest` build tag (see SetTokenEndpoint).
func SetTrainingTargetsV4URL(s string) func() {
	orig := trainingTargetsV4URL
	trainingTargetsV4URL = s
	return func() { trainingTargetsV4URL = orig }
}

// SetTrainingTargetsV4BaseURL overrides trainingTargetsV4BaseURL for tests. Returns a restore function.
// Gated behind the `polartest` build tag (see SetTokenEndpoint).
func SetTrainingTargetsV4BaseURL(s string) func() {
	orig := trainingTargetsV4BaseURL
	trainingTargetsV4BaseURL = s
	return func() { trainingTargetsV4BaseURL = orig }
}
