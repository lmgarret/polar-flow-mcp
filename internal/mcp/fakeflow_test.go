package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// fakeFlow is an httptest stand-in for flow.polar.com that serves canned wire
// responses (captured live) and records every request, so handler tests can
// assert both what went out and — for validation failures — that nothing did.
type fakeFlow struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []fakeReq
	routes map[string][]fakeResp // "METHOD /path" → responses, consumed in order (last one repeats)
}

type fakeReq struct {
	Method, Path, Body, XRequestedWith string
}

type fakeResp struct {
	status      int
	contentType string
	body        string
}

func newFakeFlow(t *testing.T) *fakeFlow {
	t.Helper()
	f := &fakeFlow{t: t, routes: map[string][]fakeResp{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	// Sport catalogue used by the sport-id checks.
	f.on("GET", "/api/sports/sports", 200, "application/json", `{"1":"RUNNING","2":"CYCLING","3":"WALKING","23":"SWIMMING"}`)
	return f
}

func (f *fakeFlow) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	key := r.Method + " " + r.URL.Path
	f.mu.Lock()
	f.reqs = append(f.reqs, fakeReq{r.Method, r.URL.Path, string(body), r.Header.Get("X-Requested-With")})
	queue := f.routes[key]
	var resp fakeResp
	ok := len(queue) > 0
	if ok {
		resp = queue[0]
		if len(queue) > 1 {
			f.routes[key] = queue[1:]
		}
	}
	f.mu.Unlock()
	if !ok {
		f.t.Errorf("unexpected request %s", key)
		w.WriteHeader(599)
		return
	}
	if resp.contentType != "" {
		w.Header().Set("Content-Type", resp.contentType)
	}
	w.WriteHeader(resp.status)
	_, _ = io.WriteString(w, resp.body)
}

// on queues a response for METHOD /path. Several calls for the same route are
// served in order; the last one keeps answering.
func (f *fakeFlow) on(method, path string, status int, contentType, body string) *fakeFlow {
	key := method + " " + path
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[key] = append(f.routes[key], fakeResp{status, contentType, body})
	return f
}

func (f *fakeFlow) client() *flow.Client {
	f.t.Helper()
	c, err := flow.NewForTesting(f.srv.URL)
	if err != nil {
		f.t.Fatalf("NewForTesting: %v", err)
	}
	return c
}

// writes returns every non-GET request.
func (f *fakeFlow) writes() []fakeReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeReq
	for _, r := range f.reqs {
		if r.Method != http.MethodGet {
			out = append(out, r)
		}
	}
	return out
}

// requests returns every request except the sport-catalogue read.
func (f *fakeFlow) requests() []fakeReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeReq
	for _, r := range f.reqs {
		if r.Path != "/api/sports/sports" {
			out = append(out, r)
		}
	}
	return out
}

// only returns the single write request, failing otherwise.
func (f *fakeFlow) onlyWrite() fakeReq {
	f.t.Helper()
	w := f.writes()
	if len(w) != 1 {
		f.t.Fatalf("writes = %+v, want exactly one", w)
	}
	return w[0]
}

// callTool runs a handler with the given arguments.
func callTool(t *testing.T, h toolHandler, args map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	res, err := h(context.Background(), req(args))
	if err != nil {
		t.Fatalf("handler returned a Go error: %v", err)
	}
	if res == nil {
		t.Fatalf("nil result")
	}
	return res
}

func resultText(res *mcpgo.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(mcpgo.TextContent); ok {
		return tc.Text
	}
	return ""
}

// assertJSONEqual compares two JSON documents semantically.
func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

// validationCase is one bad-input row: the handler must answer with a tool
// error containing wantErr and send no request except the sport catalogue
// read (a GET).
type validationCase struct {
	name    string
	args    map[string]any
	wantErr string
}

func runValidationCases(t *testing.T, mk func(*flow.Client) toolHandler, cases []validationCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			res := callTool(t, mk(ff.client()), tc.args)
			text := resultText(res)
			if !res.IsError {
				t.Fatalf("expected a tool error, got success: %s", text)
			}
			if !strings.Contains(text, tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", text, tc.wantErr)
			}
			if reqs := ff.requests(); len(reqs) != 0 {
				t.Fatalf("validation failure must not reach Flow, but sent %+v", reqs)
			}
		})
	}
}
