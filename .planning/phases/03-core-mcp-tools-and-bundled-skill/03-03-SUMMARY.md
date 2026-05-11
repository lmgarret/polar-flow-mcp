---
phase: 03-core-mcp-tools-and-bundled-skill
plan: "03"
subsystem: skill-documentation
tags: [skill, mcp, polar-flow, coaching, training-targets]

# Dependency graph
requires:
  - phase: 03-core-mcp-tools-and-bundled-skill
    plan: "01"
    provides: create_training_target tool registration and phases array shape
  - phase: 03-core-mcp-tools-and-bundled-skill
    plan: "02"
    provides: list_training_targets and delete_training_target tool registrations
provides:
  - skill/polar-coach/SKILL.md — bundled Claude skill for polar-flow-mcp
  - Trigger conditions, tool names, HR zone vocabulary, worked examples, installation paths
  - Safe degradation guidance for unreachable server and unlinked account cases
affects: [phase-04-documentation-foss-hygiene-release]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Skill file uses ## sections so it parses cleanly when uploaded to Claude.ai project files"
    - "Quick reference card at bottom for fast tool lookup"

key-files:
  created:
    - skill/polar-coach/SKILL.md
  modified: []

key-decisions:
  - "SKILL.md written after mcp.go is stable (Wave 3) so tool names match registrations verbatim"
  - "Iterative approach for marathon plans: ask-then-create per-block, not silent batch creation"
  - "Intensity labels preferred over hr_zone integers in skill guidance"
  - "Safe degradation: never fabricate results, always surface server errors to user"

patterns-established:
  - "Skill sections ordered: trigger -> tools -> HR zones -> defaults -> examples -> when-not-to -> degradation -> install"

requirements-completed: [SKILL-01, SKILL-02, SKILL-03, SKILL-04]

# Metrics
duration: 8min
completed: 2026-05-11
---

# Phase 3 Plan 03: polar-coach Skill Summary

**Bundled Claude skill (skill/polar-coach/SKILL.md, 227 lines) with HR zone table, 5 worked examples, safe degradation guidance, and both Claude Desktop/Code + Claude.ai installation paths**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-05-11T15:14:00Z
- **Completed:** 2026-05-11T15:22:10Z
- **Tasks:** 1/1
- **Files modified:** 1 created

## Accomplishments

- Created `skill/polar-coach/SKILL.md` with 227 lines covering all required sections
- All 4 tool names verified verbatim against `internal/mcp/mcp.go` registrations — no drift
- HR zone table maps all 5 labels (easy Z1, aerobic Z2, tempo Z3, threshold Z4, vo2max Z5) to coaching descriptions
- Worked examples: 5x1km threshold session (full JSON), iterative marathon plan (ask-then-create pattern), mixed-zone ladder, list+delete flow, account link check
- When-NOT-to-call section (past sessions, pace targets, power targets, device pairing, general advice)
- Safe degradation guidance: server unreachable, unlinked account (/oauth/login), 4xx/5xx errors
- Both installation paths: Claude Desktop/Code local skills directory and Claude.ai project file upload
- Quick reference card and full argument shapes at end of file

## Task Commits

1. **Task 1: Author skill/polar-coach/SKILL.md** - `6412bbe` (docs)

**Plan metadata:** _(pending final metadata commit)_

## Files Created/Modified

- `skill/polar-coach/SKILL.md` — 227-line bundled Claude skill; trigger conditions, 4 tool names verbatim, HR zone vocabulary table, defaults (10 min warmup, 5 min cooldown, 18:00 scheduled time), 5 worked examples, when-not-to-call, safe degradation, installation paths, quick reference card

## Decisions Made

- Tool names written after Wave 3 tool registrations stable; all 4 names verified via grep against `internal/mcp/mcp.go` before writing — registration is authoritative
- Marathon plan example uses iterative ask-then-create pattern (SKILL-03 T-03-03-04 mitigation: user stays in control of destructive batch operations)
- Intensity labels preferred over hr_zone integers per D-06 locking decision
- "Do NOT fabricate results" stated explicitly in both tool availability check and safe degradation sections

## Deviations from Plan

None — plan executed exactly as written. Tool names matched documented names exactly (no drift detected).

## Sections checklist

- [x] `## When to use this skill` — trigger conditions
- [x] `## Tools` — all 4 tool names verbatim in table
- [x] `### Tool availability check` — safe degradation entry point
- [x] `## HR zones and intensity vocabulary` — Z1-Z5 table with all 5 labels
- [x] `## Defaults` — warmup 10 min, cooldown 5 min, time 18:00, sport RUNNING
- [x] `## Worked examples` — 5 examples including 5x1km threshold and iterative marathon plan
- [x] `## When NOT to call tools`
- [x] `## Safe degradation` — server unreachable, unlinked account, 4xx/5xx errors
- [x] `## Installation` — Option 1 (Claude Desktop/Code) + Option 2 (Claude.ai)
- [x] `## Quick reference card`

## Issues Encountered

None.

## User Setup Required

None — documentation-only plan. No external services or environment variables.

## Next Phase Readiness

- Phase 3 complete: all 3 plans done (create_training_target, list+delete, polar-coach skill)
- Phase 4 (Documentation, FOSS Hygiene, Release) is ready to begin
- Polar API live JSON shapes still unvalidated against a live account — noted as open question in STATE.md

---

## Self-Check: PASSED

- `skill/polar-coach/SKILL.md` exists: FOUND
- Commit `6412bbe` exists: FOUND
- Line count 227 >= 150: PASS
- Tool name grep (all 4): PASS
- HR zone labels (all 5): PASS
- No tool name drift: PASS

---
*Phase: 03-core-mcp-tools-and-bundled-skill*
*Completed: 2026-05-11*
