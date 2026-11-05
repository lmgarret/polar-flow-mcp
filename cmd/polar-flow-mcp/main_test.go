package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// A route file above mcp-go's 4 MiB default body limit must still reach the
// handler: import_route accepts up to 25 MB inline.
func TestStreamableHTTPBodyLimit(t *testing.T) {
	h := newStreamableHTTPServer(server.NewMCPServer("test", "0"))

	post := func(size int) int {
		body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"_pad":"` + strings.Repeat("a", size) + `"}}`
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post(8 << 20); code == http.StatusRequestEntityTooLarge {
		t.Fatalf("8 MiB body rejected with %d; want it under the limit", code)
	}
	if code := post(maxRequestBodyBytes + 1); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over maxRequestBodyBytes: got %d, want 413", code)
	}
}
