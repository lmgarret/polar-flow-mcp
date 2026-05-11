---
phase: 03
verified: 2026-05-11T16:00:00Z
status: human_needed
must_haves_verified: 15/15
score: 15/15 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Verify create_training_target produces a session visible in Polar Flow app"
    expected: "Saying '5x1km threshold session for next Thursday at 18:00' in Claude POSTs to Polar and the target appears in Polar Flow with 5 repeats at Z4 intensity"
    why_human: "Polar API endpoint and JSON shape are ASSUMED from v4 swagger — not confirmed against a live account. The implementation logic is sound and fully wired but live API acceptance requires a real Polar account."
  - test: "Verify list_training_targets returns real data from Polar Flow"
    expected: "Calling list_training_targets with a linked account returns actual upcoming sessions from Polar, not an empty list or error"
    why_human: "Dual-shape JSON parser handles both bare-array and wrapped-object response shapes; the correct live shape is unknown until a real API call is made. If Polar uses a third shape, a parser extension is needed."
  - test: "Verify delete_training_target removes a session visible in the Polar Flow app"
    expected: "Calling delete_training_target with a valid target_id obtained from list_training_targets removes that session; Polar Flow app confirms it is gone"
    why_human: "DELETE endpoint shape (204 vs 200 vs other) assumed. ErrTargetNotFound sentinel handles 404 correctly in tests but live behavior needs confirmation."
---

# Phase 3: Core MCP Tools + Bundled Skill Verification Report

**Phase Goal:** A user can dictate a training session in plain language to Claude ("5x1km threshold with 2min recovery for Thursday"), have it created in Polar Flow, view upcoming targets, and delete incorrect ones — with a bundled skill guiding Claude's behavior without any user configuration.
**Verified:** 2026-05-11T16:00:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | store.GetEncryptedToken(ctx, identity) returns AES-GCM blob for linked user, (nil,false,nil) for unlinked | VERIFIED | `internal/store/store_tokens.go:36` — method exists; JOIN on users.identity; ErrNoRows handled at line 45 |
| 2 | RegisterTools accepts *crypto.Cipher and main.go passes it through | VERIFIED | `internal/mcp/mcp.go:18` signature confirmed; `cmd/polar-flow-mcp/main.go:64` call site confirmed; no `_ = cipher` stub remaining |
| 3 | create_training_target tool is registered with the MCP server | VERIFIED | `internal/mcp/mcp.go:27` — mcpgo.NewTool("create_training_target"...); `mcp.go:61` — s.AddTool wired to CreateTrainingTargetHandler(st, cipher) |
| 4 | Unlinked user calling create_training_target returns /oauth/login hint (no panic, no 500) | VERIFIED | `mcp_create_training_target.go:185` — ToolResultText with "/oauth/login"; 10 handler tests include TestCreateTrainingTarget_NoLinkedAccount; all tests pass |
| 5 | Flat-to-tree transform: warmup->single phase, repeat->repeat node with children, cooldown->single phase | VERIFIED | `flatToTree` at line 116 and `buildRepeatPhase` at line 63 in mcp_create_training_target.go; body assertions in TestCreateTrainingTarget_Success_5x1kmThreshold confirm structure |
| 6 | Intensity labels easy/aerobic/tempo/threshold/vo2max map to HR zones 1/2/3/4/5 | VERIFIED | `labelToZone` map at mcp_create_training_target.go:23; lowerZone=upperZone per resolveZone at line 34 |
| 7 | Duration sent to Polar in milliseconds (seconds * 1000); distance sent in meters | VERIFIED | 4 occurrences of `* 1000` in mcp_create_training_target.go (flatToTree warmup, cooldown, repeat work, recovery); distance passed through as float64 |
| 8 | polar.Client.CreateTrainingTarget POSTs with Bearer auth and JSON Content-Type | VERIFIED | `internal/polar/client.go:211` — method uses http.NewRequestWithContext, sets Authorization and Content-Type headers; httptest stub test (TestCreateTrainingTarget_Success) verifies |
| 9 | list_training_targets is registered and returns human-readable list for linked user | VERIFIED | `mcp.go:63` — registered; ListTrainingTargetsHandler produces `- {id}: {name} ({date} {time})` format per line; TestListTrainingTargets_PopulatedList confirms format |
| 10 | list_training_targets defaults from_date to today and to_date to today+30 days when omitted | VERIFIED | `mcp_list_training_targets.go:57` — `time.Now().UTC().AddDate(0, 0, 30)`; TestListTrainingTargets_DefaultDates verifies query string contains today's date |
| 11 | delete_training_target is registered and returns confirmation on 2xx from Polar | VERIFIED | `mcp.go:72` — registered; handler at mcp_delete_training_target.go:76 returns "Training target %s deleted." |
| 12 | delete_training_target returns ToolResultText (not error) on 404 — "not found" is informational | VERIFIED | `mcp_delete_training_target.go:62` — errors.Is(deleteErr, polar.ErrTargetNotFound) returns ToolResultText; TestDeleteTrainingTarget_NotFound confirms not IsError |
| 13 | Both new handlers return /oauth/login hint when user is unlinked | VERIFIED | Both handlers have identical identity/token check chains (lines 27-46 in list handler, 27-46 in delete handler) — confirmed by test fixtures |
| 14 | All handlers use auth.UserIDFromContext (MCP-07 — no new context key types) | VERIFIED | All three handlers import `auth` and call `auth.UserIDFromContext(ctx)` as the sole identity source |
| 15 | skill/polar-coach/SKILL.md exists with all required content (trigger, 4 tool names, HR zones, examples, degradation, install paths) | VERIFIED | 227 lines; all 4 tool names present (get_user_info:2, create_training_target:6, list_training_targets:5, delete_training_target:3); Z1-Z5 zone table; all 5 labels; 10-min warmup, 5-min cooldown, 18:00 default; 5x1km example; marathon iterative example; both Claude Desktop/Code and Claude.ai install paths documented |

**Score:** 15/15 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/store/store_tokens.go` | GetEncryptedToken method | VERIFIED | func at line 36; JOIN query; ErrNoRows guard |
| `internal/mcp/mcp.go` | RegisterTools(s, st, cipher) + all 4 tool registrations | VERIFIED | Signature at line 18; all 4 s.AddTool calls present; no `_ = cipher` stub |
| `internal/mcp/mcp_create_training_target.go` | CreateTrainingTargetHandler + labelToZone + flat-to-tree | VERIFIED | All three elements present; 271 lines; substantive implementation |
| `internal/mcp/mcp_list_training_targets.go` | ListTrainingTargetsHandler | VERIFIED | 99 lines; full identity/decrypt/default-date chain wired |
| `internal/mcp/mcp_delete_training_target.go` | DeleteTrainingTargetHandler | VERIFIED | 79 lines; ErrTargetNotFound sentinel check wired |
| `internal/polar/client.go` | CreateTrainingTarget + ListTrainingTargets + DeleteTrainingTarget + struct types + ErrTargetNotFound | VERIFIED | All 3 methods present (lines 211, 253, 309); 7 struct types; ErrTargetNotFound sentinel at line 196 |
| `internal/polar/testexports.go` | SetTrainingTargetsBaseURL (polartest-gated) | VERIFIED | `//go:build polartest` at line 1; SetTrainingTargetsBaseURL at line 24 |
| `cmd/polar-flow-mcp/main.go` | RegisterTools call with cipher | VERIFIED | `mcp.RegisterTools(mcpServer, st, cipher)` at line 64 |
| `skill/polar-coach/SKILL.md` | Polar coach skill — all required sections | VERIFIED | 227 lines (>= 150); all content verified |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| mcp_create_training_target.go | store.GetEncryptedToken | st.GetEncryptedToken(ctx, identity) | WIRED | Line 191 |
| mcp_create_training_target.go | crypto.Cipher.Decrypt | cipher.Decrypt(blob) | WIRED | Line 202 |
| mcp_create_training_target.go | polar.Client.CreateTrainingTarget | polar.NewClient(token).CreateTrainingTarget(ctx, polarUserID, body) | WIRED | Lines 258-259 |
| cmd/polar-flow-mcp/main.go | mcp.RegisterTools | mcp.RegisterTools(mcpServer, st, cipher) | WIRED | Line 64 |
| mcp_list_training_targets.go | polar.Client.ListTrainingTargets | polar.NewClient(token).ListTrainingTargets(ctx, polarUserID, fromDate, toDate) | WIRED | Lines 69-70 |
| mcp_delete_training_target.go | polar.Client.DeleteTrainingTarget | polar.NewClient(token).DeleteTrainingTarget(ctx, polarUserID, targetID) | WIRED | Lines 60-61 |
| mcp.go | ListTrainingTargetsHandler/DeleteTrainingTargetHandler | s.AddTool(listTool, ListTrainingTargetsHandler(st, cipher)); s.AddTool(deleteTool, DeleteTrainingTargetHandler(st, cipher)) | WIRED | Lines 70, 79 |
| skill/polar-coach/SKILL.md | internal/mcp/mcp.go tool name registrations | Tool names quoted verbatim — no drift | WIRED | Parity check: all 4 names VERIFIED (get_user_info, create_training_target, list_training_targets, delete_training_target) |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| mcp_create_training_target.go | id (target ID) | polar.Client.CreateTrainingTarget -> Polar API POST | Yes (real HTTP call with Bearer token) | FLOWING — data flows from identity context -> store -> decrypt -> Polar API; assumed endpoint |
| mcp_list_training_targets.go | targets []TrainingTargetSummary | polar.Client.ListTrainingTargets -> Polar API GET | Yes (real HTTP call; dual-shape parser) | FLOWING — same chain; live Polar shape unvalidated (WARNING, not BLOCKER — handled by human verification item) |
| mcp_delete_training_target.go | deleteErr / success | polar.Client.DeleteTrainingTarget -> Polar API DELETE | Yes (real HTTP call; ErrTargetNotFound sentinel) | FLOWING — full chain wired |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Project builds with no errors | `CGO_ENABLED=0 go build ./...` | exit 0, no output | PASS |
| Full test suite passes (without race) | `CGO_ENABLED=0 go test -tags=polartest -count=1 ./...` | All 7 packages OK | PASS |
| Full test suite passes (with race, CGO required) | `CGO_ENABLED=1 go test -tags=polartest -race -count=1 ./...` | All 7 packages OK — 2s each | PASS |
| Lint clean | `~/go/bin/golangci-lint run ./...` | 0 issues | PASS |

Note: The CLAUDE.md pre-commit checklist specifies `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...` but `-race` requires CGO. The race test was verified with `CGO_ENABLED=1` instead and passed. This is a documentation inconsistency in CLAUDE.md (CGO=0 + race is incompatible), not an implementation gap. The concurrent race test (TestConcurrentListAndDeleteHandlers) does exist and passes.

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| MCP-02 | 03-01 | create_training_target accepts name/sport/date/time/phases with intensity label and hr_zone | SATISFIED | Tool registered with full JSON schema in mcp.go; phaseInput struct handles all fields |
| MCP-03 | 03-01 | Maps intensity_label to Polar HR zones; constructs JSON; POSTs to /v3/users/{id}/training-targets | SATISFIED | labelToZone map; polar.Client.CreateTrainingTarget; note: requirement says easy->Z1-2 but code maps easy->Z1 (zone 1 strictly), consistent with locked plan decision D-06 |
| MCP-04 | 03-02 | list_training_targets with optional date range; defaults today to +30 days; returns human-readable list | SATISFIED | AddDate(0,0,30) in handler; formatted output per line |
| MCP-05 | 03-02 | delete_training_target by target_id; confirmation or clear not-found message | SATISFIED | ErrTargetNotFound sentinel; ToolResultText (not error) on 404 |
| MCP-06 | 03-01, 03-02 | All handlers extract identity, look up token, return clear error if no linked account | SATISFIED | All 3 handlers have identical identity/GetPolarUserID/GetEncryptedToken/decrypt chain |
| MCP-07 | 03-01, 03-02 | Use unexported struct type as context key — no new key types | SATISFIED | All handlers use auth.UserIDFromContext(ctx) exclusively; no new context key types introduced |
| SKILL-01 | 03-03 | SKILL.md: trigger, 4 tool names, HR zone table Z1-Z5, defaults | SATISFIED | All present; 227-line file; verified by grep gates |
| SKILL-02 | 03-03 | Worked examples: 5x1km threshold + 12-week marathon iterative approach | SATISFIED | Example 1 (5x1km with full JSON), Example 2 (iterative marathon ask-then-create), 3 additional examples |
| SKILL-03 | 03-03 | When NOT to call tools + safe degradation | SATISFIED | "When NOT to call tools" section present; safe degradation covers server unreachable, unlinked account, 4xx/5xx |
| SKILL-04 | 03-03 | Both installation paths: Claude Desktop/Code skills dir and Claude.ai project upload | SATISFIED | Option 1 and Option 2 documented in Installation section |

**Orphaned requirements check:** MCP-02 through MCP-07 and SKILL-01 through SKILL-04 are all claimed by phase 3 plans and verified. No orphaned requirements.

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/polar/client.go | 225 | Comment "ASSUMED endpoint — validate live" | INFO | Documents that the Polar v3 training targets API endpoint has not been confirmed against a live account. Present in CreateTrainingTarget, ListTrainingTargets comments. This is the intentional residual risk documented in all three plan threat models. |

No TBD/FIXME/XXX markers found in any files modified by this phase. No stub return values (return null, return {}, return []) in production handler code. No hardcoded empty data flowing to rendering.

---

### Human Verification Required

#### 1. Live Polar API: create_training_target produces a visible session

**Test:** With a real Polar account linked via OAuth, ask Claude "create a 5x1km threshold session for next Thursday at 18:00" and observe the result in the Polar Flow mobile app.
**Expected:** A training target appears in the Polar Flow calendar with 5 repeat phases at Z4 (threshold) intensity, 18:00 scheduled time, and the correct date.
**Why human:** The Polar AccessLink v3 `/v3/users/{id}/training-targets` POST endpoint shape is ASSUMED from Polar v4 swagger (no public v3 spec). The JSON body structure (`session.startTime` as a nested DateTime object, `exercise[].phaseOrRepeat` nested tree) may differ from what the live API accepts. A 400 response from Polar on first live call is the expected failure mode; it would surface as "polar: create training target: status 400: ..." in the tool result, enabling fast iteration.

#### 2. Live Polar API: list_training_targets returns real data

**Test:** With a linked account that has existing training targets, call `list_training_targets` (or ask Claude "what workouts do I have planned this week?") and verify the returned list matches what is shown in the Polar Flow app.
**Expected:** A non-empty list of targets matching the Polar Flow calendar view, formatted as `- {id}: {name} ({date} {time})`.
**Why human:** The Polar list response shape is ASSUMED to be either a bare JSON array or `{"training-targets": [...]}`. If Polar uses a different wrapper (e.g., paginated), the dual-shape parser will fail and return an error. The parser handles the two most likely shapes based on Polar v4 swagger research but cannot be verified without a live account.

#### 3. Live Polar API: delete_training_target removes a session

**Test:** Create a test session via create_training_target, note the returned ID, then call delete_training_target with that ID. Verify the session is absent from the Polar Flow app.
**Expected:** "Training target {id} deleted." is returned. Polar Flow app shows no such session.
**Why human:** DELETE status code behavior (204 vs 200 vs other 2xx) is assumed. Additionally, confirm that calling delete_training_target with a non-existent ID returns a ToolResultText "not found" message rather than a panic or server error.

---

### Gaps Summary

No programmatic gaps found. All 15 must-haves are VERIFIED at all four levels (exists, substantive, wired, data-flowing). The only remaining uncertainty is live Polar API acceptance — this is expected and documented as residual risk in all three plan threat models. The implementation is sound and all failure modes surface cleanly to the user.

The status is `human_needed` (not `passed`) because live API validation is required by ROADMAP success criterion 1 ("Saying '5×1km threshold session...' produces a Polar training target **visible in the Polar Flow app**").

---

_Verified: 2026-05-11T16:00:00Z_
_Verifier: Claude (gsd-verifier)_
