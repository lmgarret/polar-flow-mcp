---
phase: 02-oauth-link-flow-userinfo
verified: 2026-05-10T00:00:00Z
status: gaps_found
score: 10/13 must-haves verified
overrides_applied: 0
gaps:
  - truth: "GET /oauth/callback validates state via DELETE...RETURNING, requires identity match, exchanges code at polarremote.com, registers user, encrypts token, upserts users + polar_tokens, and renders 200 inline HTML"
    status: partial
    reason: "ConsumeOAuthState deletes the expired state row before checking expiry (CR-01). A caller receiving ErrExpired has already had the row destroyed; the Callback handler tells users to 'try again' but there is nothing to retry — they must restart from /oauth/login. Additionally, polar.RegisterUser sends accessToken as the member-id field (CR-02), which is semantically incorrect and a credential exposure in Polar's request logs. The review finds both issues in the flow that OAUTH-02 and OAUTH-04 depend on."
    artifacts:
      - path: "internal/store/store_oauth.go"
        issue: "ConsumeOAuthState: DELETE...RETURNING removes the row unconditionally before expiry is checked (lines 40-53). Expired rows are destroyed before ErrExpired is returned, making the Callback's 'try again' message incorrect and creating a narrow state-hijacking window."
      - path: "internal/polar/client.go"
        issue: "RegisterUser line 89: sends accessToken as member-id body field. Polar API expects the user's member ID here, not the OAuth2 bearer token. json.Marshal error is also silently discarded (body, _ :=)."
    missing:
      - "Fix ConsumeOAuthState to check expiry BEFORE DELETE (e.g., SELECT first then DELETE, or add CASE WHEN expires_at < datetime('now') THEN 1 ELSE 0 END AS is_expired to the RETURNING clause and act on it)"
      - "Fix oauth.go ErrExpired branch: message must say 'restart from /oauth/login', not 'try again'"
      - "Verify correct member-id value for Polar /v3/users endpoint against live Polar docs; fix RegisterUser accordingly"
      - "Handle json.Marshal error in RegisterUser instead of silently discarding it"

  - truth: "Replaying the same state returns 400 (state already consumed)"
    status: partial
    reason: "Replay protection works for the normal case (DELETE...RETURNING ensures second call gets ErrNotFound). However the same expired-state bug (CR-01) means: if an attacker submits a known-expired state, it gets consumed silently, potentially denying the legitimate user their state. The replay guarantee is structurally correct only for non-expired states."
    artifacts:
      - path: "internal/store/store_oauth.go"
        issue: "Expiry check post-delete means an expired state is still 'consumed' (deleted) rather than rejected cleanly. A concurrent attacker can race the expiry window to consume a valid state."
    missing:
      - "Same fix as above: move expiry check before or inside the DELETE"

  - truth: "testexports.go must not ship in production binaries (CR-03)"
    status: failed
    reason: "internal/polar/testexports.go is a regular Go source file (not _test.go), so SetTokenEndpoint and SetRegisterEndpoint are compiled into production binaries. Any code that imports internal/polar receives these mutation functions in its binary. This is an unintended expansion of the production API surface."
    artifacts:
      - path: "internal/polar/testexports.go"
        issue: "Non-test file (package polar, no _test.go suffix) exports two global-mutating functions. These ship in the production server binary."
    missing:
      - "Rename testexports.go so it is excluded from production builds, OR gate it with a build constraint (//go:build ignore or a test-only tag), OR refactor endpoint injection to use constructor-level parameters instead of package globals"
---

# Phase 2: OAuth Link Flow + UserInfo Verification Report

**Phase Goal:** OAuth Link Flow + UserInfo — users can link their Polar account via OAuth and query their linked status via MCP tool.
**Verified:** 2026-05-10T00:00:00Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | config.Load() refuses to start if POLAR_CLIENT_ID, POLAR_CLIENT_SECRET, or POLAR_REDIRECT_URL is empty | VERIFIED | config.go:114-143 — loadPolarFields() checks each env var and returns a named error; called from Load() at line 74 |
| 2 | Store can insert and consume OAuth CSRF state atomically (DELETE...RETURNING) | VERIFIED | store_oauth.go:40-43 — uses writeDB.QueryRowContext with DELETE...RETURNING. ConsumeOAuthState is atomic for the non-expired case. BLOCKER: expiry check is post-delete (CR-01) |
| 3 | Store can upsert a user keyed by identity, storing polar_user_id as TEXT | VERIFIED | store_users.go:13-24 — INSERT...ON CONFLICT(identity) DO UPDATE SET polar_user_id = excluded.polar_user_id |
| 4 | Store can upsert an encrypted token blob keyed on the user's identity | VERIFIED | store_tokens.go:14-28 — subquery (SELECT id FROM users WHERE identity = ?) resolves FK; ON CONFLICT(user_id) DO UPDATE overwrites blob |
| 5 | Sentinel errors ErrNotFound and ErrExpired are exported from the store package | VERIFIED | store_oauth.go:15-20 — both exported as package-level var with //nolint:gochecknoglobals |
| 6 | GET /oauth/login redirects (302) to flow.polar.com with response_type=code, client_id, redirect_uri, scope=accesslink.read_all, and a 64-char hex state | VERIFIED | oauth.go:71-78 — all required params built with url.Values; state is hex.EncodeToString of 32 crypto/rand bytes (64 chars); redirects to authorizeURL |
| 7 | After /oauth/login a row exists in pending_auth bound to the proxy identity with expires_at ~= now+10min | VERIFIED | oauth.go:63-68 — CreateOAuthState called with time.Now().UTC().Add(stateTTL) where stateTTL=10min; identity from UserIDFromContext |
| 8 | GET /oauth/callback validates state, requires identity match, exchanges code, registers user, encrypts token, upserts users+polar_tokens, renders 200 HTML | PARTIAL | The sequence exists in oauth.go:82-158 and is fully wired. Two sub-issues block full verification: (a) expired-state delete-before-check bug (CR-01) means expiry path has incorrect UX and potential race; (b) RegisterUser sends accessToken as member-id (CR-02) which is semantically wrong and a credential exposure |
| 9 | Replaying the same state returns 400 | PARTIAL | Normal replay returns 400 via ErrNotFound (DELETE...RETURNING ensures single-use). Expired-state race window exists per CR-01 — attacker can race a known-expired state to consume it |
| 10 | Polar registration 409 is treated as success and x_user_id from token exchange is used as polar_user_id | VERIFIED | client.go:104-106 returns (0, nil) on 409; oauth.go:136 always uses strconv.FormatInt(tr.XUserID, 10) regardless of RegisterUser's return value |
| 11 | get_user_info MCP tool is registered on the server | VERIFIED | mcp.go:17-25 — RegisterTools calls s.AddTool with mcpgo.NewTool("get_user_info", ...) and GetUserInfoHandler(st) |
| 12 | Linked user receives a tool result containing their proxy identity and Polar user ID | VERIFIED | mcp.go:49-52 — returns "Polar account linked.\nIdentity: %s\nPolar user ID: %s" |
| 13 | Unlinked user receives a tool result containing a clear hint to visit /oauth/login | VERIFIED | mcp.go:42-45 — returns "No Polar account linked to your identity (%s). Visit /oauth/login to link your Polar account." |

**Score:** 10/13 truths verified (3 partial/failed)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/config/config.go` | PolarClientID/PolarClientSecret/PolarRedirectURL + loadPolarFields() | VERIFIED | All three fields present; loadPolarFields called from Load() at line 74 |
| `internal/store/store_oauth.go` | CreateOAuthState, ConsumeOAuthState, ErrNotFound, ErrExpired | VERIFIED (with issue) | All present; expiry-check ordering bug (CR-01) noted |
| `internal/store/store_users.go` | UpsertUser, GetPolarUserID | VERIFIED | Exact signatures match plan |
| `internal/store/store_tokens.go` | UpsertToken with FK subquery | VERIFIED | Subquery present; ON CONFLICT updates blob and key_version |
| `internal/polar/client.go` | TokenResponse, ExchangeCode, RegisterUser, defaultHTTPClient | VERIFIED (with issue) | All present; member-id semantic error (CR-02) |
| `internal/oauth/oauth.go` | Handlers struct + NewHandlers + Login + Callback + HTML helpers | VERIFIED | All present; ErrExpired UX message is incorrect (says "try again" after consuming the state) |
| `internal/crypto/bytes_provider.go` | BytesKeyProvider for wiring cfg.EncryptionKey | VERIFIED | NewBytesKeyProvider and Key() with 32-byte validation present |
| `internal/mcp/mcp.go` | RegisterTools(s, st) + GetUserInfoHandler | VERIFIED | Both present; tool registered with get_user_info name |
| `cmd/polar-flow-mcp/main.go` | cipher construction + oauth.NewHandlers + mcp.RegisterTools(mcpServer, st) | VERIFIED | All three wiring points confirmed at lines 53, 64, 97 |
| `internal/polar/testexports.go` | Test-only injection helpers | FAILED | Non-test file ships in production binary (CR-03) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| config.Load() | loadPolarFields(cfg) | called after loadAuthFields | WIRED | config.go line 74 |
| store.ConsumeOAuthState | pending_auth table | DELETE...RETURNING on writeDB | WIRED | store_oauth.go:40-43 |
| store.UpsertToken | polar_tokens FK to users.id | subquery (SELECT id FROM users WHERE identity = ?) | WIRED | store_tokens.go:17 |
| oauth.go LoginHandler | store.CreateOAuthState | h.st field | WIRED | oauth.go:65 |
| oauth.go CallbackHandler | ConsumeOAuthState + ExchangeCode + RegisterUser + Encrypt + UpsertUser + UpsertToken | sequential calls | WIRED | oauth.go:91-156 — all six calls present in sequence |
| cmd/polar-flow-mcp/main.go | oauth.NewHandlers(cfg, st, cipher) | route registration | WIRED | main.go:97 |
| mcp.RegisterTools | auth.UserIDFromContext + st.GetPolarUserID | tool handler closure | WIRED | mcp.go:31-36 |
| cmd/polar-flow-mcp/main.go | mcp.RegisterTools(mcpServer, st) | two-arg call | WIRED | main.go:64 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| internal/mcp/mcp.go GetUserInfoHandler | polarUserID | store.GetPolarUserID → readDB.QueryRowContext → SELECT polar_user_id FROM users | DB query confirmed at store_users.go:31-43 | FLOWING |
| internal/oauth/oauth.go Callback | blob | cipher.Encrypt([]byte(tr.AccessToken)) → UpsertToken | Encrypted real token from Polar token exchange | FLOWING |

### Behavioral Spot-Checks

Step 7b: SKIPPED — server requires live env vars and DB to run; all behavioral paths are covered by the existing test suite (CGO_ENABLED=1 go test -race -count=1 ./... confirmed green per SUMMARY).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| OAUTH-01 | 02-01, 02-02 | GET /oauth/login: 32-byte state, 10-min expiry, redirect to Polar with scope=accesslink.read_all | SATISFIED | oauth.go:48-78 |
| OAUTH-02 | 02-02 | GET /oauth/callback validates state (present, found, not expired, identity bound), deletes state (single-use) | PARTIAL | State validation present; expired-state delete-before-check (CR-01) means expiry UX is wrong |
| OAUTH-03 | 02-02 | Exchange code via POST https://polarremote.com/v2/oauth2/token with HTTP Basic auth | SATISFIED | client.go:53-81; oauth.go:123; Basic auth via SetBasicAuth |
| OAUTH-04 | 02-02 | Register user via POST https://www.polaraccesslink.com/v3/users; 409 = idempotent success | PARTIAL | client.go:88-118 — 409 handled; but member-id field sends accessToken (CR-02) — semantically wrong, credential exposure |
| OAUTH-05 | 02-01, 02-02 | Encrypt token, upsert in polar_tokens with key_version=1; respond 200 with "Polar account linked" | SATISFIED | oauth.go:145-158; htmlSuccess renders "Polar account linked"; keyVersion=1 at oauth.go:33 |
| MCP-01 | 02-03 | get_user_info tool: returns linked account or "no account linked" message | SATISFIED | mcp.go:17-53; three response paths (linked, unlinked, no identity) all implemented and tested |

**Orphaned requirements:** None — all six requirement IDs from PLAN frontmatter are covered.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/store/store_oauth.go | 40-53 | DELETE before expiry check — expiry validated after row is destroyed | BLOCKER | Incorrect UX ("try again" is a lie), potential state-hijacking window (CR-01) |
| internal/polar/client.go | 89 | accessToken sent as member-id body field | BLOCKER | Credential exposure in Polar's request logs; semantically wrong per Polar API docs (CR-02) |
| internal/polar/client.go | 89 | json.Marshal error silently discarded (body, _ :=) | WARNING | If Marshal fails, sends empty body to Polar API; misleading error from Polar |
| internal/polar/testexports.go | 1-17 | Non-test file exports global mutators — ships in production binary | BLOCKER | SetTokenEndpoint/SetRegisterEndpoint are part of production binary API surface (CR-03) |
| internal/oauth/oauth.go | 131 | RegisterUser return value discarded with _ — never validated against tr.XUserID | WARNING | Latent logic error; currently harmless because contract says always use tr.XUserID (CR-04) |
| internal/mcp/mcp.go | 38-39 | Raw DB error text returned to MCP client | WARNING | Information disclosure — internal schema details exposed to Claude (WR-04) |
| cmd/polar-flow-mcp/main.go | 73-75 | panic in WithHTTPContextFunc — no recovery middleware confirmed | WARNING | Potential server crash on wiring bug rather than 500 response (WR-05) |

### Human Verification Required

None — all behavioral paths are verifiable programmatically.

### Gaps Summary

Three gaps block full goal achievement:

**Gap 1 (BLOCKER) — Expired-state delete-before-check (CR-01, affects OAUTH-02 and OAUTH-05)**

`ConsumeOAuthState` runs `DELETE...RETURNING` unconditionally, then checks expiry on the returned row. This means expired state rows are consumed (deleted) even when they should be rejected. The consequence is twofold: (a) `oauth.go` line 98 tells users to "try again" but the state is gone — they must restart from `/oauth/login`; (b) in the expiry window, an attacker who submits a known-expired state destroys it before the legitimate user's callback can use it.

Files: `internal/store/store_oauth.go` (fix: move expiry check before delete) and `internal/oauth/oauth.go` (fix: correct the ErrExpired branch message).

**Gap 2 (BLOCKER) — RegisterUser sends accessToken as member-id (CR-02, affects OAUTH-04)**

`polar.RegisterUser` sends the OAuth2 bearer access token as the `member-id` JSON field. Polar's `/v3/users` registration endpoint expects the user's Polar member identifier, not a bearer token. Sending the access token here exposes the credential in Polar's server-side request logs. The `json.Marshal` error is also silently discarded. This must be verified against live Polar API docs before Phase 3 depends on token-based API calls.

File: `internal/polar/client.go` line 89.

**Gap 3 (BLOCKER) — testexports.go ships in production binary (CR-03)**

`internal/polar/testexports.go` is not a `_test.go` file, so `SetTokenEndpoint` and `SetRegisterEndpoint` are compiled into production binaries. This unintentionally expands the production API surface with global-mutating functions.

File: `internal/polar/testexports.go` — rename with build constraint or refactor to constructor-level injection.

---

_Verified: 2026-05-10T00:00:00Z_
_Verifier: Claude (gsd-verifier)_
