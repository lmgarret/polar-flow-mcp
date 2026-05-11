# Phase 3: Core MCP Tools + Bundled Skill - Pattern Map

**Mapped:** 2026-05-11
**Files analyzed:** 10 (7 Go files + 3 test files + 1 skill file)
**Analogs found:** 10 / 10

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/mcp/mcp.go` | provider/registry | request-response | self (modify existing) | exact — add cipher param + 3 AddTool calls |
| `internal/mcp/mcp_create_training_target.go` | handler/controller | request-response | `internal/mcp/mcp.go` (GetUserInfoHandler) | exact role+flow |
| `internal/mcp/mcp_list_training_targets.go` | handler/controller | request-response | `internal/mcp/mcp.go` (GetUserInfoHandler) | exact role+flow |
| `internal/mcp/mcp_delete_training_target.go` | handler/controller | request-response | `internal/mcp/mcp.go` (GetUserInfoHandler) | exact role+flow |
| `internal/polar/client.go` | service/client | request-response | self (modify existing) | exact — add 3 methods to *Client |
| `internal/polar/testexports.go` | test-utility | — | self (modify existing) | exact — add SetTrainingTargetsBaseURL |
| `internal/store/store_tokens.go` | store | CRUD | `internal/store/store_users.go` (GetPolarUserID) | exact role+flow |
| `internal/mcp/mcp_test.go` | test | request-response | self (extend existing) | exact — same openTestStore/resultText helpers |
| `internal/polar/client_training_targets_test.go` | test | request-response | `internal/polar/client_test.go` | exact — httptest.Server + SetXxxEndpoint pattern |
| `skill/polar-coach/SKILL.md` | documentation | — | none (new top-level dir) | no analog |

---

## Pattern Assignments

### `internal/mcp/mcp.go` — signature update

**Change:** Add `cipher *crypto.Cipher` parameter to `RegisterTools`; add import for `crypto` package; call three new handler registrations.

**Analog:** self (lines 1–25 of `internal/mcp/mcp.go`)

**Current signature** (line 17):
```go
func RegisterTools(s *server.MCPServer, st *store.Store) {
```

**Updated signature pattern:**
```go
import (
    "context"
    "fmt"

    mcpgo "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"

    "github.com/lm/polar-flow-mcp/internal/auth"
    "github.com/lm/polar-flow-mcp/internal/crypto"
    "github.com/lm/polar-flow-mcp/internal/store"
)

func RegisterTools(s *server.MCPServer, st *store.Store, cipher *crypto.Cipher) {
    // existing get_user_info registration unchanged
    s.AddTool(getUserInfoTool, GetUserInfoHandler(st))

    // Phase 3 additions:
    s.AddTool(createTool, CreateTrainingTargetHandler(st, cipher))
    s.AddTool(listTool,   ListTrainingTargetsHandler(st, cipher))
    s.AddTool(deleteTool, DeleteTrainingTargetHandler(st, cipher))
}
```

**Call site update in `cmd/polar-flow-mcp/main.go`** (line 64):
```go
// Before:
mcp.RegisterTools(mcpServer, st)

// After:
mcp.RegisterTools(mcpServer, st, cipher)
// cipher is already constructed at line 53:
// cipher := crypto.NewCipher(crypto.NewBytesKeyProvider(cfg.EncryptionKey))
```

---

### `internal/mcp/mcp_create_training_target.go` (handler, request-response)

**Analog:** `internal/mcp/mcp.go` — `GetUserInfoHandler` (lines 27–54)

**Imports pattern** — follow mcp.go import block, add encoding/json and polar:
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strings"
    "time"

    mcpgo "github.com/mark3labs/mcp-go/mcp"

    "github.com/lm/polar-flow-mcp/internal/auth"
    "github.com/lm/polar-flow-mcp/internal/crypto"
    "github.com/lm/polar-flow-mcp/internal/polar"
    "github.com/lm/polar-flow-mcp/internal/store"
)
```

**Handler closure signature pattern** — copy from `GetUserInfoHandler` (mcp.go lines 28–30):
```go
func CreateTrainingTargetHandler(
    st *store.Store,
    cipher *crypto.Cipher,
) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
```

**Identity + store lookup pattern** — copy verbatim from mcp.go lines 31–47:
```go
        identity, ok := auth.UserIDFromContext(ctx)
        if !ok {
            return mcpgo.NewToolResultError("no identity in context — auth middleware not applied"), nil
        }

        polarUserID, found, err := st.GetPolarUserID(ctx, identity)
        if err != nil {
            return mcpgo.NewToolResultError("database error: " + err.Error()), nil
        }
        if !found {
            return mcpgo.NewToolResultText(fmt.Sprintf(
                "No Polar account linked to your identity (%s). "+
                    "Visit /oauth/login to link your Polar account.",
                identity,
            )), nil
        }
```

**Token decrypt pattern** — new in Phase 3; extends the identity pattern:
```go
        blob, found, err := st.GetEncryptedToken(ctx, identity)
        if err != nil {
            return mcpgo.NewToolResultError("database error: " + err.Error()), nil
        }
        if !found {
            return mcpgo.NewToolResultText(fmt.Sprintf(
                "No token stored for your identity (%s). "+
                    "Visit /oauth/login to re-link your Polar account.",
                identity,
            )), nil
        }
        tokenBytes, err := cipher.Decrypt(blob)
        if err != nil {
            return mcpgo.NewToolResultError("token decryption error: " + err.Error()), nil
        }
```

**Tool schema registration** — `mcp.NewTool` pattern from mcp.go lines 18–23, extended with WithArray:
```go
createTool := mcpgo.NewTool("create_training_target",
    mcpgo.WithDescription(
        "Create a Polar Flow training target with phases (warmup, repeat intervals, cooldown). "+
            "Constructs the full phases array; Claude provides the phase structure.",
    ),
    mcpgo.WithString("name",
        mcpgo.Required(),
        mcpgo.Description("Session name, max 45 chars (e.g., \"5×1km Threshold\")"),
    ),
    mcpgo.WithString("date",
        mcpgo.Required(),
        mcpgo.Description("Scheduled date, ISO 8601 (e.g., \"2026-05-15\")"),
    ),
    mcpgo.WithString("sport",
        mcpgo.Description("Sport type (default: RUNNING)"),
    ),
    mcpgo.WithString("time",
        mcpgo.Description("Scheduled time HH:MM (default: 18:00)"),
    ),
    mcpgo.WithArray("phases",
        mcpgo.Required(),
        mcpgo.Description("Ordered list of phases: warmup, repeat blocks, cooldown"),
        mcpgo.Items(map[string]any{
            "type": "object",
            "properties": map[string]any{
                "type":       map[string]any{"type": "string", "enum": []string{"warmup", "repeat", "cooldown"}},
                "duration_s": map[string]any{"type": "integer", "description": "Duration in seconds (warmup/cooldown)"},
                "reps":       map[string]any{"type": "integer", "description": "Repeat count (repeat type only)"},
                "goal":       map[string]any{"type": "object"},
                "intensity":  map[string]any{"type": "object"},
                "recovery":   map[string]any{"type": "object"},
            },
            "required": []string{"type"},
        }),
    ),
)
```

**Label-to-zone mapping** — package-level var, looked up in handler:
```go
// labelToZone maps coaching intensity labels (D-06) to Polar HR zones 1–5.
//
//nolint:gochecknoglobals
var labelToZone = map[string]int{
    "easy":      1,
    "aerobic":   2,
    "tempo":     3,
    "threshold": 4,
    "vo2max":    5,
}

func resolveZone(label string, hrZone int) (int, bool) {
    if hrZone >= 1 && hrZone <= 5 {
        return hrZone, true
    }
    if z, ok := labelToZone[label]; ok {
        return z, true
    }
    return 0, false
}
```

**Error result return convention** — always `(mcpgo.NewToolResultError(msg), nil)` for user-visible errors; never `(nil, err)`:
```go
// From mcp.go line 33:
return mcpgo.NewToolResultError("no identity in context — auth middleware not applied"), nil

// Success:
return mcpgo.NewToolResultText("Training target created. ID: " + id), nil
```

---

### `internal/mcp/mcp_list_training_targets.go` (handler, request-response)

**Analog:** `internal/mcp/mcp.go` — `GetUserInfoHandler` (lines 27–54)

**Identical patterns:** handler closure signature, identity extraction, GetPolarUserID lookup, token decrypt, error result convention — copy verbatim from create handler above.

**Tool schema** — no array parameter; optional date strings:
```go
listTool := mcpgo.NewTool("list_training_targets",
    mcpgo.WithDescription(
        "List upcoming Polar Flow training targets. "+
            "Defaults to today through +30 days if no date range is provided.",
    ),
    mcpgo.WithString("from_date",
        mcpgo.Description("Start date ISO 8601 (default: today)"),
    ),
    mcpgo.WithString("to_date",
        mcpgo.Description("End date ISO 8601 (default: today +30 days)"),
    ),
)
```

**Date default pattern** — stdlib time.Parse, project convention:
```go
fromDate := req.GetString("from_date", time.Now().Format("2006-01-02"))
toDate   := req.GetString("to_date",   time.Now().AddDate(0, 0, 30).Format("2006-01-02"))
```

---

### `internal/mcp/mcp_delete_training_target.go` (handler, request-response)

**Analog:** `internal/mcp/mcp.go` — `GetUserInfoHandler` (lines 27–54)

**Identical patterns:** handler closure signature, identity extraction, token decrypt, error result convention.

**Tool schema:**
```go
deleteTool := mcpgo.NewTool("delete_training_target",
    mcpgo.WithDescription("Delete a Polar Flow training target by its ID."),
    mcpgo.WithString("target_id",
        mcpgo.Required(),
        mcpgo.Description("Training target ID returned by create_training_target or list_training_targets"),
    ),
)
```

**404 handling pattern** — based on polar/client.go lines 111–113 (StatusConflict → idempotent success):
```go
// In polar.Client.DeleteTrainingTarget:
if resp.StatusCode == http.StatusNotFound {
    return fmt.Errorf("polar: training target not found: %s", targetID)
}
// In handler: check error string, return friendly ToolResultText (not Error):
if strings.Contains(err.Error(), "not found") {
    return mcpgo.NewToolResultText(
        fmt.Sprintf("Training target %s not found. It may have already been deleted.", targetID),
    ), nil
}
```

---

### `internal/polar/client.go` — three new methods (service, request-response)

**Analog:** self — `RegisterUser` method (lines 92–125) is the closest pattern: POST with JSON body, Bearer auth, non-OK status error with body excerpt.

**Package-level endpoint vars pattern** — copy from lines 31–41:
```go
//nolint:gochecknoglobals
var (
    // existing vars ...
    tokenEndpoint    = "https://polarremote.com/v2/oauth2/token"
    registerEndpoint = "https://www.polaraccesslink.com/v3/users"

    // Phase 3 additions:
    trainingTargetsBaseURL = "https://www.polaraccesslink.com/v3/users"
    // used as: trainingTargetsBaseURL + "/" + polarUserID + "/training-targets"
)
```

**Method pattern** — copy from `RegisterUser` (lines 92–125); key elements:
```go
func (c *Client) CreateTrainingTarget(
    ctx context.Context,
    polarUserID string,
    req CreateTrainingTargetRequest,
) (string, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return "", fmt.Errorf("polar: marshal training target: %w", err)
    }
    url := trainingTargetsBaseURL + "/" + polarUserID + "/training-targets"
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
    if err != nil {
        return "", fmt.Errorf("polar: build create training target request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Accept", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+c.bearerToken)

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return "", fmt.Errorf("polar: create training target: %w", err)
    }
    defer func() { _ = resp.Body.Close() }()

    if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
        errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return "", fmt.Errorf("polar: create training target: status %d: %s", resp.StatusCode, errBody)
    }
    // parse response for created ID...
}
```

**Non-OK error pattern** — copy from `RegisterUser` lines 114–117:
```go
errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
return ..., fmt.Errorf("polar: <operation>: status %d: %s", resp.StatusCode, errBody)
```

**Struct types** — internal to polar package; thin, exported only enough for tests:
```go
// CreateTrainingTargetRequest is the POST body for /v3/users/{id}/training-targets.
// Field names are ASSUMED from Polar v4 swagger schema (see RESEARCH.md A1–A8).
type CreateTrainingTargetRequest struct {
    Session  TrainingSessionTarget `json:"session"`
    Exercise []ExerciseTarget      `json:"exercise"`
}

type TrainingSessionTarget struct {
    Name      string   `json:"name"`
    StartTime DateTime `json:"startTime"`
}

type DateTime struct {
    Year  int `json:"year"`
    Month int `json:"month"`
    Day   int `json:"day"`
    Hour  int `json:"hour"`
    Min   int `json:"min"`
    Sec   int `json:"sec"`
}

type ExerciseTarget struct {
    Idx           int64           `json:"idx"`
    Type          string          `json:"type"` // "PHASED"
    PhaseOrRepeat []PhaseOrRepeat `json:"phaseOrRepeat,omitempty"`
}

type PhaseOrRepeat struct {
    Name          string          `json:"name"`
    ChangeType    string          `json:"changeType"` // "AUTOMATIC" or "MANUAL"
    Goal          PhaseGoal       `json:"goal"`
    Intensity     *PhaseIntensity `json:"intensity,omitempty"`
    RepeatCount   *int            `json:"repeatCount,omitempty"`
    PhaseOrRepeat []PhaseOrRepeat `json:"phaseOrRepeat,omitempty"`
}

type PhaseGoal struct {
    Type     string   `json:"type"`              // "DURATION" | "DISTANCE" | "MANUAL"
    Duration *int64   `json:"duration,omitempty"` // milliseconds
    Distance *float64 `json:"distance,omitempty"` // meters
}

type PhaseIntensity struct {
    Type      string `json:"type"`               // "HEART_RATE_ZONES" | "NONE"
    LowerZone *int   `json:"lowerZone,omitempty"` // 1–5
    UpperZone *int   `json:"upperZone,omitempty"` // 1–5
}
```

---

### `internal/polar/testexports.go` — add new endpoint setter (test-utility)

**Analog:** self — existing `SetTokenEndpoint` / `SetRegisterEndpoint` (lines 1–20)

**Copy pattern exactly:**
```go
//go:build polartest

package polar

// SetTokenEndpoint overrides tokenEndpoint for tests. Returns a restore function.
func SetTokenEndpoint(s string) func() {
    orig := tokenEndpoint
    tokenEndpoint = s
    return func() { tokenEndpoint = orig }
}

// SetTrainingTargetsBaseURL overrides trainingTargetsBaseURL for tests. Returns a restore function.
// Gated behind the `polartest` build tag so it does NOT ship in production binaries (CR-03).
func SetTrainingTargetsBaseURL(s string) func() {
    orig := trainingTargetsBaseURL
    trainingTargetsBaseURL = s
    return func() { trainingTargetsBaseURL = orig }
}
```

---

### `internal/store/store_tokens.go` — add GetEncryptedToken method (store, CRUD)

**Analog:** `internal/store/store_users.go` — `GetPolarUserID` (lines 27–45)

**Import additions** — copy store_users.go import block (lines 1–9), already has `database/sql`, `errors`, `fmt`:
```go
import (
    "context"
    "database/sql"
    "errors"
    "fmt"
)
```

**Method pattern** — copy directly from `GetPolarUserID` (lines 29–45), adapting table join and Scan type:
```go
// GetEncryptedToken returns the encrypted token blob for identity.
// Returns (nil, false, nil) if no token row exists for the identity.
// Returns a non-nil error only on database failure.
func (s *Store) GetEncryptedToken(ctx context.Context, identity string) ([]byte, bool, error) {
    var blob []byte
    err := s.readDB.QueryRowContext(ctx,
        `SELECT pt.encrypted_token
         FROM polar_tokens pt
         JOIN users u ON u.id = pt.user_id
         WHERE u.identity = ?`,
        identity,
    ).Scan(&blob)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, false, nil
    }
    if err != nil {
        return nil, false, fmt.Errorf("store: get encrypted token: %w", err)
    }
    return blob, true, nil
}
```

Key differences from `GetPolarUserID`:
- Scans into `[]byte` not `sql.NullString` (BLOB column, never NULL if row exists)
- Uses `s.readDB` (read pool) — same as `GetPolarUserID`
- JOIN on `users.id = polar_tokens.user_id` — same JOIN that `TestUpsertToken_Insert` already uses (store_tokens_test.go lines 41–44)

---

### `internal/mcp/mcp_test.go` — extend with new handler tests (test)

**Analog:** self — existing test file (lines 1–131)

**Test helpers to reuse verbatim** (lines 19–39):
```go
func openTestStore(t *testing.T) *store.Store { ... }  // lines 19–28
func resultText(result *mcpgo.CallToolResult) string { ... }  // lines 31–39
```

**Test structure pattern** — copy from `TestGetUserInfo_Linked` (lines 43–74):
```go
func TestCreateTrainingTarget_NoIdentity(t *testing.T) {
    st := openTestStore(t)
    cipher := newTestCipher(t) // helper constructs crypto.NewCipher with fixed 32-byte key
    ctx := context.Background() // no identity

    handler := mcp.CreateTrainingTargetHandler(st, cipher)
    result, err := handler(ctx, mcpgo.CallToolRequest{})
    if err != nil {
        t.Fatalf("handler returned unexpected error: %v", err)
    }
    if !result.IsError {
        t.Fatalf("expected error result for missing identity, got: %s", resultText(result))
    }
}
```

**Context injection pattern** — copy from mcp_test.go line 45:
```go
ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")
```

**IsError check convention** — mcp_test.go lines 60–63 (check `result.IsError` boolean, not error return):
```go
if result.IsError {
    t.Fatalf("expected non-error result, got error result with text: %s", resultText(result))
}
```

---

### `internal/polar/client_training_targets_test.go` (test, request-response)

**Analog:** `internal/polar/client_test.go` — full file (lines 1–179)

**Package declaration** — external test package, same as client_test.go (line 1):
```go
package polar_test
```

**Imports pattern** — copy from client_test.go lines 3–16:
```go
import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/lm/polar-flow-mcp/internal/polar"
)
```

**httptest.Server stub pattern** — copy from `TestExchangeCode_Success` (lines 19–57):
```go
func TestCreateTrainingTarget_Success(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            t.Errorf("expected POST, got %s", r.Method)
        }
        if got := r.Header.Get("Authorization"); got != "Bearer tok" {
            t.Errorf("Authorization = %q, want 'Bearer tok'", got)
        }
        // Assert JSON body fields...
        body, _ := io.ReadAll(r.Body)
        var payload map[string]any
        if err := json.Unmarshal(body, &payload); err != nil {
            t.Errorf("body not valid JSON: %v", err)
        }
        w.WriteHeader(http.StatusCreated)
        _ = json.NewEncoder(w).Encode(map[string]any{"id": "99"})
    }))
    defer ts.Close()

    restore := polar.SetTrainingTargetsBaseURL(ts.URL)
    t.Cleanup(restore)

    client := polar.NewClient("tok")
    // call method, assert result...
}
```

**Non-OK status test pattern** — copy from `TestExchangeCode_NonOKStatus` (lines 77–100):
```go
func TestCreateTrainingTarget_ErrorStatus(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusBadRequest)
        _, _ = fmt.Fprint(w, "invalid request")
    }))
    defer ts.Close()
    // ...
    if !strings.Contains(err.Error(), "status 400") { ... }
}
```

---

### `skill/polar-coach/SKILL.md` (documentation)

**Analog:** none — new top-level directory with no compilation dependency. No pattern to copy from existing codebase.

**Structure dictated by CONTEXT.md D-11** (authoritative, not a pattern match):
1. Trigger description — when Claude should invoke these tools
2. Tool list — exact names from `mcp.NewTool("name", ...)` calls (copy verbatim to avoid D-07 drift)
3. HR zone table — Z1–Z5 with `easy/aerobic/tempo/threshold/vo2max` labels (D-06)
4. Worked examples — 5×1km threshold session with full phases array (from CONTEXT.md `<specifics>`)
5. When NOT to call tools — guard conditions
6. Safe degradation — check server availability first; if unavailable, tell user + deployment link (D-12)
7. Installation paths — Claude Desktop/Code skills directory + Claude.ai project upload (D-13)

Tool names to use verbatim in SKILL.md (from `mcp.NewTool` first argument):
- `get_user_info`
- `create_training_target`
- `list_training_targets`
- `delete_training_target`

---

## Shared Patterns

### Identity Extraction
**Source:** `internal/auth/auth.go` lines 25–28, used in `internal/mcp/mcp.go` lines 31–34
**Apply to:** All four handler files (`mcp_create_training_target.go`, `mcp_list_training_targets.go`, `mcp_delete_training_target.go`, `mcp.go` existing handler)
```go
identity, ok := auth.UserIDFromContext(ctx)
if !ok {
    return mcpgo.NewToolResultError("no identity in context — auth middleware not applied"), nil
}
```

### Error Result Convention
**Source:** `internal/mcp/mcp.go` lines 33, 38, 42
**Apply to:** All handler files
- User-visible recoverable error: `return mcpgo.NewToolResultError("message"), nil`
- User-visible informational non-error: `return mcpgo.NewToolResultText("message"), nil`
- Transport/framework failure (rare): `return nil, err`
- Never: `return nil, userFacingError`

### HTTP Request Construction (noctx linter)
**Source:** `internal/polar/client.go` lines 59, 98
**Apply to:** All new `*Client` methods in `internal/polar/client.go`
```go
req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
// NEVER: http.NewRequest(...) — noctx linter fails
// NEVER: http.Get(...) or http.Post(...) — noctx linter fails
```

### Error Body Pattern for Non-OK Polar Responses
**Source:** `internal/polar/client.go` lines 73–75 and 114–116
**Apply to:** All new `*Client` methods
```go
body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
return ..., fmt.Errorf("polar: <operation>: status %d: %s", resp.StatusCode, body)
```

### Deferred Body Close
**Source:** `internal/polar/client.go` lines 71, 108
**Apply to:** All new `*Client` methods
```go
defer func() { _ = resp.Body.Close() }()
```

### Store Read Pool Usage
**Source:** `internal/store/store_users.go` line 31 (`s.readDB.QueryRowContext`)
**Apply to:** `GetEncryptedToken` in `store_tokens.go`
- Read queries: `s.readDB.QueryRowContext` / `s.readDB.QueryContext`
- Write queries: `s.writeDB.ExecContext`

### Test Build Tag
**Source:** `internal/polar/testexports.go` line 1
**Apply to:** All `testexports.go` files; also test helper files that call test-only exports
```go
//go:build polartest
```

### Test Store Helper
**Source:** `internal/mcp/mcp_test.go` lines 19–28 (`openTestStore`)
**Apply to:** All new `*_test.go` files in `internal/mcp/`
```go
func openTestStore(t *testing.T) *store.Store {
    t.Helper()
    dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
    st, err := store.Open(dsn)
    if err != nil {
        t.Fatalf("openTestStore: %v", err)
    }
    t.Cleanup(func() { _ = st.Close() })
    return st
}
```

### slog for Logging
**Source:** `internal/auth/auth.go` lines 47–52, `cmd/polar-flow-mcp/main.go` lines 31, 48
**Apply to:** All new handler and client files
```go
slog.Error("operation failed", "error", err, "identity", identity)
slog.Warn("unexpected state", "detail", detail)
// NEVER: fmt.Println, log.Printf
```

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `skill/polar-coach/SKILL.md` | documentation | — | No existing skill files in codebase; structure fully specified by CONTEXT.md D-11 through D-13 |

---

## Pitfalls (pattern-level reminders for planner)

| Pitfall | Affected Files | Guard |
|---------|---------------|-------|
| Flat phases array → nested Polar tree transform | `mcp_create_training_target.go` | Each `repeat` tool element maps to a Polar repeat node with `repeatCount` and child `phaseOrRepeat` array |
| `duration_s` (seconds) must become milliseconds in Polar body | `mcp_create_training_target.go`, `internal/polar/client.go` structs | `polarMs := durationS * 1000` |
| `RegisterTools` signature change breaks `main.go` | `internal/mcp/mcp.go`, `cmd/polar-flow-mcp/main.go` | Update both in same task |
| `GetEncryptedToken` missing on Store | `internal/store/store_tokens.go` | Add before any handler that calls it |
| SKILL.md tool name drift | `skill/polar-coach/SKILL.md` | Copy names verbatim from `mcp.NewTool("name", ...)` first args |
| `noctx` linter on new Polar HTTP calls | `internal/polar/client.go` | Always `http.NewRequestWithContext(ctx, ...)` |

---

## Metadata

**Analog search scope:** `internal/mcp/`, `internal/polar/`, `internal/store/`, `internal/auth/`, `internal/crypto/`, `cmd/`
**Files read:** 12 source files + 2 planning docs
**Pattern extraction date:** 2026-05-11
