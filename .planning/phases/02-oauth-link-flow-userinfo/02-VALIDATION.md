---
phase: 2
slug: oauth-link-flow-userinfo
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-05
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + `net/http/httptest` |
| **Config file** | none (Go tests are discovered automatically) |
| **Quick run command** | `CGO_ENABLED=0 go test -count=1 ./internal/store/... ./internal/polar/... ./internal/oauth/... ./internal/mcp/... ./internal/config/...` |
| **Full suite command** | `CGO_ENABLED=0 go test -race -count=1 ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `CGO_ENABLED=0 go test -count=1 ./internal/...`
- **After every plan wave:** Run `CGO_ENABLED=0 go test -race -count=1 ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| config fields | 01 | 1 | D-01 | — | Fails startup if POLAR_CLIENT_ID/SECRET/REDIRECT_URL missing | unit | `CGO_ENABLED=0 go test ./internal/config/...` | ❌ W0 | ⬜ pending |
| store_oauth.go | 01 | 1 | OAUTH-02 | CSRF state replay | DELETE...RETURNING single-use; ErrNotFound/ErrExpired on miss | unit | `CGO_ENABLED=0 go test ./internal/store/...` | ❌ W0 | ⬜ pending |
| store_users.go | 01 | 1 | D-08, D-10 | — | UpsertUser / GetPolarUserID round-trip | unit | `CGO_ENABLED=0 go test ./internal/store/...` | ❌ W0 | ⬜ pending |
| store_tokens.go | 01 | 1 | OAUTH-05 | Token leakage | Encrypted blob stored; decrypted on demand | unit | `CGO_ENABLED=0 go test ./internal/store/...` | ❌ W0 | ⬜ pending |
| polar ExchangeCode | 02 | 1 | OAUTH-03 | — | 200 → token extracted; non-200 → error returned | unit (httptest mock) | `CGO_ENABLED=0 go test ./internal/polar/...` | ❌ W0 | ⬜ pending |
| polar RegisterUser | 02 | 1 | OAUTH-04 | — | 200 → polar_user_id extracted; 409 → idempotent success | unit (httptest mock) | `CGO_ENABLED=0 go test ./internal/polar/...` | ❌ W0 | ⬜ pending |
| oauth LoginHandler | 02 | 2 | OAUTH-01 | CSRF generation | 302 to Polar URL; state param present; DB row created | httptest | `CGO_ENABLED=0 go test ./internal/oauth/...` | ❌ W0 | ⬜ pending |
| oauth CallbackHandler | 02 | 2 | OAUTH-02..05 | CSRF validation; identity mismatch | Valid flow → 200 HTML + DB rows; invalid state → 400; expired → 400; mismatch → 400 | httptest | `CGO_ENABLED=0 go test ./internal/oauth/...` | ❌ W0 | ⬜ pending |
| get_user_info tool | 03 | 2 | MCP-01 | — | Linked → polar_user_id; unlinked → hint message (not panic) | unit | `CGO_ENABLED=0 go test ./internal/mcp/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/config/config_test.go` extension — table-driven: each missing Polar env var → specific error message
- [ ] `internal/store/store_oauth_test.go` — CreateOAuthState + ConsumeOAuthState (happy path, missing state, expired state)
- [ ] `internal/store/store_users_test.go` — UpsertUser + GetPolarUserID
- [ ] `internal/store/store_tokens_test.go` — UpsertToken (UpsertUser prerequisite enforced)
- [ ] `internal/polar/client_test.go` — ExchangeCode + RegisterUser with httptest mock servers
- [ ] `internal/oauth/oauth_test.go` — LoginHandler redirect + CallbackHandler end-to-end with mocked Polar endpoints
- [ ] `internal/mcp/mcp_test.go` — get_user_info linked and unlinked cases

*All test files use existing Go stdlib testing + httptest — no new framework install needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Visiting `/oauth/login` actually redirects to `flow.polar.com` | OAUTH-01 | Requires live Polar developer credentials | Set POLAR_CLIENT_ID/SECRET/REDIRECT_URL; run server; visit /oauth/login; verify redirect to flow.polar.com |
| Full OAuth round-trip with real Polar account | OAUTH-01..05 | Requires live Polar account and developer app registration | Complete Polar OAuth flow; verify polar_tokens row exists with correct nonce||ciphertext layout |
| Calling `get_user_info` in Claude after linking | MCP-01 | Requires live Claude + MCP connection | Link Polar account; call get_user_info tool; verify polar_user_id in response |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
