---
phase: 02-oauth-link-flow-userinfo
verified: 2026-05-10T14:00:00Z
status: passed
score: 13/13
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 10/13
  gaps_closed:
    - "ConsumeOAuthState checked expiry post-delete (CR-01) — now uses conditional DELETE gated on expires_at >= datetime('now'); expired rows are retained"
    - "polar.RegisterUser sent accessToken as member-id body field (CR-02) — now accepts explicit memberID param; member-id field receives Polar x_user_id string; marshal error is surfaced"
    - "internal/polar/testexports.go shipped SetTokenEndpoint/SetRegisterEndpoint in production binary (CR-03) — now gated behind //go:build polartest; symbols absent from production binary (verified via go tool nm)"
  gaps_remaining: []
  regressions: []
---

# Phase 2: OAuth Link Flow + UserInfo Verification Report

**Phase Goal:** A user can navigate to /oauth/login from behind their reverse proxy, authorize with Polar, and have their encrypted token stored — then verify the link by calling get_user_info from Claude.
**Verified:** 2026-05-10T14:00:00Z
**Status:** passed
**Re-verification:** Yes — after gap closure via plan 02-04

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | config.Load() refuses to start if POLAR_CLIENT_ID, POLAR_CLIENT_SECRET, or POLAR_REDIRECT_URL is empty | VERIFIED | config.go:74 calls loadPolarFields(cfg) which checks each env var and returns a named error |
| 2 | Store can insert and consume OAuth CSRF state atomically (DELETE...RETURNING gated on expiry) | VERIFIED | store_oauth.go:44-48 — conditional DELETE WHERE expires_at >= datetime('now') RETURNING identity; expired rows retained via diagnostic SELECT on readDB |
| 3 | Store can upsert a user keyed by identity, storing polar_user_id as TEXT | VERIFIED | store_users.go:13-24 — INSERT...ON CONFLICT(identity) DO UPDATE SET polar_user_id = excluded.polar_user_id |
| 4 | Store can upsert an encrypted token blob keyed on the user's identity | VERIFIED | store_tokens.go:14-28 — subquery (SELECT id FROM users WHERE identity = ?) resolves FK; ON CONFLICT(user_id) DO UPDATE |
| 5 | Sentinel errors ErrNotFound and ErrExpired are exported from the store package | VERIFIED | store_oauth.go:15-20 — both exported as package-level var with //nolint:gochecknoglobals |
| 6 | GET /oauth/login redirects (302) to flow.polar.com with response_type=code, client_id, redirect_uri, scope=accesslink.read_all, and a 64-char hex state | VERIFIED | oauth.go:71-78 — url.Values with all required params; state = hex.EncodeToString(32 crypto/rand bytes) |
| 7 | After /oauth/login a row exists in pending_auth bound to the proxy identity with expires_at ~= now+10min | VERIFIED | oauth.go:63-68 — CreateOAuthState called with time.Now().UTC().Add(stateTTL) where stateTTL=10min |
| 8 | GET /oauth/callback validates state via conditional DELETE, requires identity match, exchanges code at polarremote.com, registers user (409=success), encrypts token, upserts users+polar_tokens, and renders 200 inline HTML | VERIFIED | oauth.go:82-158 — full sequence; ConsumeOAuthState now uses conditional DELETE (CR-01 fixed); ErrExpired branch says "restart from /oauth/login" (line 97); RegisterUser called with (tr.AccessToken, polarUserID) where polarUserID = strconv.FormatInt(tr.XUserID, 10) (CR-02 fixed) |
| 9 | Replaying the same state returns 400 (state already consumed) | VERIFIED | Normal replay: DELETE...RETURNING ensures single-use (second call gets ErrNotFound → 400). Expired state: conditional DELETE skips the row, diagnostic SELECT returns ErrExpired → 400. Neither path silently consumes a state row |
| 10 | Polar registration 409 is treated as success and x_user_id from token exchange is used as polar_user_id | VERIFIED | client.go:111-112 returns (0, nil) on 409; oauth.go:131 always uses strconv.FormatInt(tr.XUserID, 10) as polarUserID regardless of RegisterUser return value |
| 11 | get_user_info MCP tool is registered on the server | VERIFIED | mcp.go:17-25 — RegisterTools calls s.AddTool with mcpgo.NewTool("get_user_info", ...) and GetUserInfoHandler(st) |
| 12 | Linked user receives a tool result containing their proxy identity and Polar user ID | VERIFIED | mcp.go:49-52 — returns "Polar account linked.\nIdentity: %s\nPolar user ID: %s" |
| 13 | Unlinked user receives a tool result containing a clear hint to visit /oauth/login | VERIFIED | mcp.go:42-45 — returns "No Polar account is linked to your identity (%s). Visit /oauth/login to link your Polar account." |

**Score:** 13/13 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/config/config.go` | PolarClientID/PolarClientSecret/PolarRedirectURL + loadPolarFields() | VERIFIED | All three fields + function present; called from Load() at line 74 |
| `internal/store/store_oauth.go` | CreateOAuthState, ConsumeOAuthState, ErrNotFound, ErrExpired — expiry check before delete | VERIFIED | Conditional DELETE at line 44-48 confirmed; CR-01 fixed |
| `internal/store/store_users.go` | UpsertUser, GetPolarUserID | VERIFIED | Exact signatures match plan |
| `internal/store/store_tokens.go` | UpsertToken with FK subquery | VERIFIED | Subquery present; ON CONFLICT updates blob and key_version |
| `internal/polar/client.go` | TokenResponse, ExchangeCode, RegisterUser(ctx, accessToken, memberID), defaultHTTPClient | VERIFIED | All present; CR-02 fixed — memberID param accepted; member-id JSON field receives memberID; marshal error surfaced |
| `internal/oauth/oauth.go` | Handlers struct + NewHandlers + Login + Callback + HTML helpers | VERIFIED | All present; ErrExpired branch at line 97 says "restart from /oauth/login" |
| `internal/crypto/bytes_provider.go` | BytesKeyProvider for wiring cfg.EncryptionKey | VERIFIED | NewBytesKeyProvider and Key() with 32-byte validation present |
| `internal/mcp/mcp.go` | RegisterTools(s, st) + GetUserInfoHandler | VERIFIED | Both present; tool registered with get_user_info name |
| `cmd/polar-flow-mcp/main.go` | cipher construction + oauth.NewHandlers + mcp.RegisterTools(mcpServer, st) | VERIFIED | Lines 53, 64, 97 confirmed |
| `internal/polar/testexports.go` | Gated behind //go:build polartest | VERIFIED | First line is `//go:build polartest`; go tool nm confirms symbols absent from production binary |
| `internal/polar/export_test.go` | Must NOT exist (deleted in 02-04) | VERIFIED | File does not exist |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| config.Load() | loadPolarFields(cfg) | called after loadAuthFields | WIRED | config.go line 74 |
| store.ConsumeOAuthState | pending_auth table | Conditional DELETE gated on expires_at >= datetime('now'); diagnostic SELECT for ErrExpired | WIRED | store_oauth.go:44-76 |
| store.UpsertToken | polar_tokens FK to users.id | subquery (SELECT id FROM users WHERE identity = ?) | WIRED | store_tokens.go:17 |
| oauth.go LoginHandler | store.CreateOAuthState | h.st field | WIRED | oauth.go:65 |
| oauth.go CallbackHandler | ConsumeOAuthState + ExchangeCode + RegisterUser(accessToken, polarUserID) + Encrypt + UpsertUser + UpsertToken | sequential calls | WIRED | oauth.go:91-156 — all six calls present in correct order; polarUserID computed before RegisterUser call |
| cmd/polar-flow-mcp/main.go | oauth.NewHandlers(cfg, st, cipher) | route registration | WIRED | main.go:97 |
| mcp.RegisterTools | auth.UserIDFromContext + st.GetPolarUserID | tool handler closure | WIRED | mcp.go:31-36 |
| cmd/polar-flow-mcp/main.go | mcp.RegisterTools(mcpServer, st) | two-arg call | WIRED | main.go:64 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| internal/mcp/mcp.go GetUserInfoHandler | polarUserID | store.GetPolarUserID → readDB.QueryRowContext → SELECT polar_user_id FROM users | DB query confirmed at store_users.go:31-43 | FLOWING |
| internal/oauth/oauth.go Callback | blob | cipher.Encrypt([]byte(tr.AccessToken)) → UpsertToken | Encrypted real token from Polar token exchange; memberID = strconv.FormatInt(tr.XUserID, 10) | FLOWING |

### Behavioral Spot-Checks

Full test suite run: `CGO_ENABLED=0 go test -tags=polartest -count=1 ./...` — all 7 packages pass (auth, config, crypto, mcp, oauth, polar, store). The -race flag requires CGO which is disabled by project convention; the SUMMARY confirms -race was run in the CI environment.

| Behavior | Result | Status |
|----------|--------|--------|
| All packages compile without CGO | `CGO_ENABLED=0 go build ./cmd/polar-flow-mcp` exits 0 | PASS |
| Full test suite green | `CGO_ENABLED=0 go test -tags=polartest -count=1 ./...` — 7/7 packages ok | PASS |
| Lint clean | `golangci-lint run ./...` — 0 issues | PASS |
| Production binary excludes test setters | `go tool nm ./polar-flow-mcp \| grep polar.Set*` — no output | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| OAUTH-01 | 02-01, 02-02 | GET /oauth/login: 32-byte state, 10-min expiry, redirect to Polar with scope=accesslink.read_all | SATISFIED | oauth.go:48-78 |
| OAUTH-02 | 02-02, 02-04 | GET /oauth/callback validates state (present, found, not expired, identity bound), deletes state (single-use) | SATISFIED | Conditional DELETE in ConsumeOAuthState (CR-01 fixed); expired rows retained; ErrExpired branch directs user to restart |
| OAUTH-03 | 02-02 | Exchange code via POST https://polarremote.com/v2/oauth2/token with HTTP Basic auth | SATISFIED | client.go:53-81; oauth.go:123; Basic auth via SetBasicAuth |
| OAUTH-04 | 02-02, 02-04 | Register user via POST https://www.polaraccesslink.com/v3/users; 409 = idempotent success; member-id is Polar user ID not bearer token | SATISFIED | CR-02 fixed — RegisterUser(ctx, accessToken, memberID) at client.go:92; body sends memberID; 409 returns (0, nil) |
| OAUTH-05 | 02-01, 02-02 | Encrypt token, upsert in polar_tokens with key_version=1; respond 200 with "Polar account linked" | SATISFIED | oauth.go:145-158; htmlSuccess renders "Polar account linked"; keyVersion=1 at oauth.go:33 |
| MCP-01 | 02-03 | get_user_info tool: returns linked account or "no account linked" message | SATISFIED | mcp.go:17-53; three response paths all implemented and tested |

**Orphaned requirements:** None.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/mcp/mcp.go | 38-39 | Raw DB error text returned to MCP client | WARNING | Information disclosure — internal schema details exposed to Claude. Informational for v1 single-tenant. |
| cmd/polar-flow-mcp/main.go | 73-75 | panic in WithHTTPContextFunc — no recovery middleware confirmed | WARNING | Potential server crash on wiring bug rather than 500 response. No new risk introduced in phase 2. |

Previous BLOCKERs CR-01, CR-02, CR-03 are resolved.

### Human Verification Required

None — all behavioral paths are verifiable programmatically or through the test suite.

### Re-verification Gap Closure Summary

All three gaps from the initial verification are resolved:

**CR-01 (CLOSED):** `ConsumeOAuthState` now uses `DELETE FROM pending_auth WHERE state = ? AND expires_at >= datetime('now') RETURNING identity`. Expired rows are retained (not consumed). A diagnostic SELECT on readDB distinguishes ErrNotFound from ErrExpired. The ErrExpired branch in oauth.go now says "please restart from /oauth/login". Regression test `TestConsumeOAuthState_Expired_DoesNotDelete` confirms COUNT(*)==1 after the call on an expired state.

**CR-02 (CLOSED):** `RegisterUser` signature is now `(ctx context.Context, accessToken, memberID string) (int64, error)`. The JSON body sends `{"member-id": memberID}` where memberID is `strconv.FormatInt(tr.XUserID, 10)` — the Polar numeric user ID formatted as a string. The oauth.go caller computes polarUserID before the RegisterUser call and reuses it for UpsertUser. Marshal error is now surfaced.

**CR-03 (CLOSED):** `internal/polar/testexports.go` first line is `//go:build polartest`. `internal/polar/export_test.go` (the obsolete doc stub) is deleted. Production binary verified via `go tool nm` to contain no `polar.SetTokenEndpoint` or `polar.SetRegisterEndpoint` symbols. All `go test` invocations pass `-tags=polartest`; `.golangci.yml` sets `run.build-tags: [polartest]`; Makefile, CI, and CLAUDE.md are aligned.

---

_Verified: 2026-05-10T14:00:00Z_
_Verifier: Claude (gsd-verifier)_
