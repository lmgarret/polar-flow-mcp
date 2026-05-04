// Package auth_test provides integration tests for the auth middleware using httptest.
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/config"
)

const (
	testSecret   = "super-secret-value"
	testIdentity = "alice@example.com"
	testHeader   = "Remote-User"
)

// downstreamOK is a simple handler that records whether it was called and returns 200.
func downstreamOK(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

// newRequest builds an httptest.Request with the given secret and identity headers.
// Pass empty string to omit a header.
func newRequest(secret, identity string) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if secret != "" {
		r.Header.Set(config.ProxySecretHeader, secret)
	}
	if identity != "" {
		r.Header.Set(testHeader, identity)
	}
	return r
}

// Test 1: Request with correct secret and valid identity header → 200 from downstream handler.
func TestMiddleware_ValidRequest_CallsDownstream(t *testing.T) {
	t.Parallel()

	called := false
	handler := auth.Middleware(testSecret, testHeader, downstreamOK(&called))

	r := newRequest(testSecret, testIdentity)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("downstream handler was not called")
	}
}

// Test 2: Request with missing secret header → 403, downstream NOT called.
func TestMiddleware_MissingSecret_Returns403(t *testing.T) {
	t.Parallel()

	called := false
	handler := auth.Middleware(testSecret, testHeader, downstreamOK(&called))

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	// No secret header set.
	r.Header.Set(testHeader, testIdentity)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("downstream handler must not be called when secret is missing")
	}
}

// Test 3: Request with wrong secret value → 403, downstream NOT called.
func TestMiddleware_WrongSecret_Returns403(t *testing.T) {
	t.Parallel()

	called := false
	handler := auth.Middleware(testSecret, testHeader, downstreamOK(&called))

	r := newRequest("wrong-secret", testIdentity)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("downstream handler must not be called when secret is wrong")
	}
}

// Test 4: Request with correct secret but empty identity header → 403 with warning log.
func TestMiddleware_EmptyIdentity_Returns403(t *testing.T) {
	t.Parallel()

	called := false
	handler := auth.Middleware(testSecret, testHeader, downstreamOK(&called))

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r.Header.Set(config.ProxySecretHeader, testSecret)
	// Identity header deliberately omitted.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("downstream handler must not be called when identity is missing")
	}
}

// Test 5: Request with correct secret and valid identity → identity available via UserIDFromContext.
func TestMiddleware_ValidRequest_InjectsIdentity(t *testing.T) {
	t.Parallel()

	var capturedID string
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			t.Error("UserIDFromContext returned false — identity not injected")
			return
		}
		capturedID = id
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.Middleware(testSecret, testHeader, downstream)
	r := newRequest(testSecret, testIdentity)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if capturedID != testIdentity {
		t.Errorf("expected identity %q, got %q", testIdentity, capturedID)
	}
}

// Test 6: UserIDFromContext on context without identity key returns ("", false).
func TestUserIDFromContext_MissingKey_ReturnsFalse(t *testing.T) {
	t.Parallel()

	id, ok := auth.UserIDFromContext(context.Background())
	if ok {
		t.Error("expected ok=false on empty context, got true")
	}
	if id != "" {
		t.Errorf("expected empty id on empty context, got %q", id)
	}
}

// Test 7: Two concurrent requests with different identities produce different UserIDFromContext values.
// Verifies no shared state between goroutines (run with -race to catch races).
func TestMiddleware_ConcurrentRequests_IsolatedIdentity(t *testing.T) {
	t.Parallel()

	const identityA = "alice@example.com"
	const identityB = "bob@example.com"

	var (
		wg        sync.WaitGroup
		capturedA string
		capturedB string
		startGate = make(chan struct{})
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		r := newRequest(testSecret, identityA)
		w := httptest.NewRecorder()
		captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-startGate
			id, _ := auth.UserIDFromContext(r.Context())
			capturedA = id
			w.WriteHeader(http.StatusOK)
		})
		auth.Middleware(testSecret, testHeader, captureHandler).ServeHTTP(w, r)
	}()

	go func() {
		defer wg.Done()
		r := newRequest(testSecret, identityB)
		w := httptest.NewRecorder()
		captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-startGate
			id, _ := auth.UserIDFromContext(r.Context())
			capturedB = id
			w.WriteHeader(http.StatusOK)
		})
		auth.Middleware(testSecret, testHeader, captureHandler).ServeHTTP(w, r)
	}()

	// Release both goroutines simultaneously.
	close(startGate)
	wg.Wait()

	if capturedA != identityA {
		t.Errorf("goroutine A: expected identity %q, got %q", identityA, capturedA)
	}
	if capturedB != identityB {
		t.Errorf("goroutine B: expected identity %q, got %q", identityB, capturedB)
	}
	if capturedA == capturedB {
		t.Errorf("concurrent goroutines produced same identity — context isolation broken")
	}
}
