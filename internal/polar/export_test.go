// Package polar: export_test.go documents that SetTokenEndpoint and SetRegisterEndpoint
// are exported for test injection in both polar and external test packages.
// The actual functions live in testexports.go (non-test file) so they are accessible
// from test binaries of other packages (e.g., internal/oauth).
package polar
