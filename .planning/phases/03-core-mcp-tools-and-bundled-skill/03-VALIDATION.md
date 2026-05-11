---
phase: 3
slug: core-mcp-tools-and-bundled-skill
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-11
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard `go test ./...` |
| **Quick run command** | `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./internal/mcp/...` |
| **Full suite command** | `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./internal/mcp/...`
- **After every plan wave:** Run `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 3-01-01 | 01 | 0 | MCP-06 | — | store.GetEncryptedToken returns only the calling user's token | unit | `CGO_ENABLED=0 go test -tags=polartest -run TestGetEncryptedToken ./internal/store/...` | ❌ W0 | ⬜ pending |
| 3-01-02 | 01 | 1 | MCP-02 | — | create_training_target constructs correct Polar API JSON from flat input | unit | `CGO_ENABLED=0 go test -tags=polartest -run TestCreateTrainingTarget ./internal/mcp/...` | ❌ W0 | ⬜ pending |
| 3-01-03 | 01 | 1 | MCP-03 | — | HR zone label maps correctly (easy→Z1, threshold→Z4, vo2max→Z5) | unit | `CGO_ENABLED=0 go test -tags=polartest -run TestMapIntensityLabel ./internal/mcp/...` | ❌ W0 | ⬜ pending |
| 3-01-04 | 01 | 1 | MCP-07 | — | create_training_target returns clear error when user has no linked account | unit | `CGO_ENABLED=0 go test -tags=polartest -run TestCreateTrainingTargetNoAccount ./internal/mcp/...` | ❌ W0 | ⬜ pending |
| 3-02-01 | 02 | 1 | MCP-04 | — | list_training_targets returns human-readable output scoped to calling user | unit | `CGO_ENABLED=0 go test -tags=polartest -run TestListTrainingTargets ./internal/mcp/...` | ❌ W0 | ⬜ pending |
| 3-02-02 | 02 | 1 | MCP-05 | — | delete_training_target returns confirmation for valid ID, clear error for invalid | unit | `CGO_ENABLED=0 go test -tags=polartest -run TestDeleteTrainingTarget ./internal/mcp/...` | ❌ W0 | ⬜ pending |
| 3-02-03 | 02 | 1 | MCP-06 | — | no data races under concurrent simulated user requests | race | `CGO_ENABLED=0 go test -tags=polartest -race -count=1 -run TestConcurrent ./internal/mcp/...` | ❌ W0 | ⬜ pending |
| 3-03-01 | 03 | 1 | SKILL-01 | — | SKILL.md contains all 4 tool names and HR zone table | manual | inspect skill/polar-coach/SKILL.md | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/mcp/tools_test.go` — stubs for MCP-02, MCP-03, MCP-04, MCP-05, MCP-06, MCP-07
- [ ] `internal/store/store_test.go` (extend) — TestGetEncryptedToken for MCP-06
- [ ] `internal/mcp/testdata/` — mock Polar API response fixtures

*Existing go test infrastructure covers the framework; only test files are missing.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| SKILL.md renders correctly in Claude UI | SKILL-01 | No automated Claude UI test | Open Claude, install skill, verify tool list appears |
| Polar training target appears in Polar Flow app | MCP-02 | Live Polar account required | Create target via tool, check Polar Flow mobile/web |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
