---
phase: 2
plan: "02-03"
subsystem: mcp-tools
tags: [mcp, tdd, get_user_info, tool-registration, phase-gate]
dependency_graph:
  requires: [02-02-SUMMARY.md]
  provides: [get_user_info-tool, mcp-RegisterTools-with-store]
  affects: [03-01-PLAN.md]
tech_stack:
  added: []
  patterns: [tdd-red-green, export-handler-for-test, tool-handler-closure, context-identity-extraction]
key_files:
  created:
    - internal/mcp/mcp_test.go
  modified:
    - internal/mcp/mcp.go
    - cmd/polar-flow-mcp/main.go
decisions:
  - "GetUserInfoHandler exported (not unexported getUserInfoHandler) — consistent with plan and allows cross-package test invocation without live server"
  - "result.IsError check after nil guard requires explicit return after t.Fatal to satisfy SA5011 staticcheck"
  - "/oauth/login appears twice in mcp.go (tool description + handler text) — both occurrences are intentional"
metrics:
  duration: "5 min"
  completed_date: "2026-05-10"
  tasks_completed: 1
  files_created: 1
  files_modified: 2
---

# Phase 2 Plan 03: get_user_info MCP Tool Summary

**One-liner:** get_user_info MCP tool registered with exported handler closure; identity from context only (never from tool params); linked/unlinked/no-identity cases covered; Phase 2 gate fully green.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Implement get_user_info MCP tool + tests + update main.go call site + phase-gate verify | 8bb788f | internal/mcp/mcp.go, internal/mcp/mcp_test.go, cmd/polar-flow-mcp/main.go |

## What Was Built

### Task 1: get_user_info MCP Tool

**internal/mcp/mcp.go** — replaced stub entirely:

- `RegisterTools(s *server.MCPServer, st *store.Store)` — registers `get_user_info` tool via `s.AddTool`
- `GetUserInfoHandler(st *store.Store)` — exported handler closure factory; `auth.UserIDFromContext` extracts identity from context (T-02-03-01 mitigation: tool params ignored with `_`); `st.GetPolarUserID` looks up Polar account; three response paths: error result (no identity), text result with `/oauth/login` hint (unlinked), text result with identity + polar_user_id (linked)
- Import alias `mcpgo "github.com/mark3labs/mcp-go/mcp"` avoids collision with local package name

**internal/mcp/mcp_test.go** (package mcp_test):

- `TestGetUserInfo_Linked` — opens in-memory store, UpsertUser("alice", "12345"), calls handler with "alice" identity; asserts non-error result, text contains "alice", "12345", "linked"
- `TestGetUserInfo_NotLinked` — empty store, "bob" identity; asserts non-error result, text contains "bob", "/oauth/login", "no polar account"
- `TestGetUserInfo_NoIdentity` — no identity in context; asserts IsError=true, text contains "identity"
- Nil guard + explicit `return` after `t.Fatal` to satisfy SA5011 staticcheck

**cmd/polar-flow-mcp/main.go**: `mcp.RegisterTools(mcpServer)` → `mcp.RegisterTools(mcpServer, st)`

### Phase 2 Gate Results

```
CGO_ENABLED=0 go build ./...          → exit 0
CGO_ENABLED=1 go test -race -count=1 ./... → exit 0 (all packages)
golangci-lint run ./...               → 0 issues
```

All packages:
```
ok  github.com/lm/polar-flow-mcp/internal/auth    1.017s
ok  github.com/lm/polar-flow-mcp/internal/config  1.017s
ok  github.com/lm/polar-flow-mcp/internal/crypto  1.022s
ok  github.com/lm/polar-flow-mcp/internal/mcp     1.091s
ok  github.com/lm/polar-flow-mcp/internal/oauth   1.163s
ok  github.com/lm/polar-flow-mcp/internal/polar   1.035s
ok  github.com/lm/polar-flow-mcp/internal/store   1.400s
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Added explicit `return` after `t.Fatal` in nil guards**
- **Found during:** Task 1 lint run
- **Issue:** `if result == nil { t.Fatal(...) }` followed by `result.IsError` triggered SA5011 (staticcheck) — `t.Fatal` does not terminate the goroutine from the linter's flow-analysis perspective.
- **Fix:** Added `return` immediately after each `t.Fatal("handler returned nil result")` call in all three test functions.
- **Files modified:** internal/mcp/mcp_test.go
- **Commit:** 8bb788f

## Known Stubs

None.

## Threat Flags

None — no new network endpoints. The tool handler reads identity exclusively from context (T-02-03-01), uses two-value UserIDFromContext assertion (T-02-03-03), and the RegisterTools signature change is enforced by build (T-02-03-04). Database error messages are wrapped by the store package before reaching the handler (T-02-03-02).

## Self-Check: PASSED

- internal/mcp/mcp.go: FOUND
- internal/mcp/mcp_test.go: FOUND
- cmd/polar-flow-mcp/main.go: FOUND (modified)
- Commit 8bb788f: FOUND
