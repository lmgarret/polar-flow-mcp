---
phase: 2
plan: "02-01"
subsystem: config+store
tags: [config, store, oauth, sqlite, tdd]
dependency_graph:
  requires: [01-03-SUMMARY.md]
  provides: [polar-config-fields, store-oauth-state, store-upsert-user, store-upsert-token]
  affects: [02-02-PLAN.md, 02-03-PLAN.md]
tech_stack:
  added: []
  patterns: [tdd-red-green, delete-returning-atomic, upsert-on-conflict, fk-subquery]
key_files:
  created:
    - internal/store/store_oauth.go
    - internal/store/store_users.go
    - internal/store/store_tokens.go
    - internal/store/store_oauth_test.go
    - internal/store/store_users_test.go
    - internal/store/store_tokens_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
decisions:
  - "ErrNotFound and ErrExpired are sentinel errors exported from store package"
  - "ConsumeOAuthState uses DELETE...RETURNING for atomic single-use; expiry checked on returned row"
  - "UpsertToken resolves user FK via (SELECT id FROM users WHERE identity=?) subquery"
  - "polar_user_id stored as TEXT per schema ground truth"
metrics:
  duration: "12 min"
  completed_date: "2026-05-10"
  tasks_completed: 2
  files_created: 6
  files_modified: 2
---

# Phase 2 Plan 01: Config + Store OAuth/User/Token Foundations Summary

**One-liner:** Fail-closed Polar OAuth config validation and five atomic store methods (CSRF state CRUD + user/token upserts) forming the data layer for Phase 2 OAuth handlers.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Extend Config with three Polar OAuth fields | aa54381 | internal/config/config.go, internal/config/config_test.go |
| 2 | Add OAuth state, user, and token store methods | 5fed640 | internal/store/store_oauth.go, store_users.go, store_tokens.go + 3 test files |

## What Was Built

### Task 1: Config Extension

Added three fields to `Config` struct and a new `loadPolarFields()` validator called after `loadAuthFields()` in `Load()`:

- `PolarClientID` — from `POLAR_CLIENT_ID` env var
- `PolarClientSecret` — from `POLAR_CLIENT_SECRET` env var
- `PolarRedirectURL` — from `POLAR_REDIRECT_URL` env var

`Load()` fails with an actionable error naming the missing env var if any of the three are absent. Existing tests updated to call `setValidPolarEnv(t)` so they can reach their intended failure points. Four new tests added.

### Task 2: Store Methods

**store_oauth.go:**
- `ErrNotFound`, `ErrExpired` — exported sentinel errors
- `CreateOAuthState(ctx, state, identity, expiresAt)` — inserts into `pending_auth`
- `ConsumeOAuthState(ctx, state)` — `DELETE...RETURNING` on `writeDB`; returns `ErrNotFound` if absent, `ErrExpired` if past expiry (row deleted in both cases)

**store_users.go:**
- `UpsertUser(ctx, identity, polarUserID)` — `INSERT...ON CONFLICT(identity) DO UPDATE`
- `GetPolarUserID(ctx, identity)` — returns `("", false, nil)` for missing/empty, `(id, true, nil)` for found

**store_tokens.go:**
- `UpsertToken(ctx, identity, encryptedBlob, keyVersion)` — resolves `user_id` via FK subquery; `ON CONFLICT(user_id) DO UPDATE` overwrites blob and version

## Deviations from Plan

None — plan executed exactly as written.

The plan acceptance criteria for `grep -c "loadPolarFields(cfg)"` states "at least 2 (definition + call site)" but the function definition signature is `func loadPolarFields(cfg *Config) error` which does not literally contain `loadPolarFields(cfg)`. The call site at Load() line 74 matches. Both the definition and call site exist in config.go — the criteria description was imprecise about the grep pattern, not the requirement.

## Test Results

```
ok  github.com/lm/polar-flow-mcp/internal/config  0.003s
ok  github.com/lm/polar-flow-mcp/internal/store   0.037s
```

Lint: `0 issues` on both packages.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints or auth paths introduced. All SQL uses parameterized queries (? placeholders).

## Self-Check: PASSED

- internal/config/config.go: FOUND
- internal/config/config_test.go: FOUND
- internal/store/store_oauth.go: FOUND
- internal/store/store_users.go: FOUND
- internal/store/store_tokens.go: FOUND
- internal/store/store_oauth_test.go: FOUND
- internal/store/store_users_test.go: FOUND
- internal/store/store_tokens_test.go: FOUND
- Commit aa54381: FOUND
- Commit 5fed640: FOUND
