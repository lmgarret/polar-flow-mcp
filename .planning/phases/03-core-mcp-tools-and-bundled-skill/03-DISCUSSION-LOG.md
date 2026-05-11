# Phase 3: Core MCP Tools + Bundled Skill - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-11
**Phase:** 3-Core MCP Tools + Bundled Skill
**Areas discussed:** Polar API shapes, create_training_target params, Intensity label → HR zone mapping, Skill file content focus

---

## Polar API Shapes

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — I have real payloads | Shapes confirmed against live Polar account | |
| No — build against the docs/spec | Use public AccessLink v3 docs; accept potential shape corrections during testing | ✓ |
| Partially — some shapes confirmed | Some endpoints confirmed, others not | |

**User's choice:** Build against the public Polar AccessLink v3 docs/spec — shapes not confirmed live.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Use the public AccessLink v3 docs | Researcher fetches training-targets section | ✓ |
| I'll paste the relevant endpoint docs | User provides specific payload examples | |

**User's choice:** Use public docs (`https://www.polar.com/accesslink-api/#training-targets`).

---

| Option | Description | Selected |
|--------|-------------|----------|
| Wrap Polar calls in one Client method each | Thin internal structs; shape corrections only touch the client method | ✓ |
| Mirror Polar's JSON directly in tool params | Tool parameters match Polar's exact JSON; any correction touches both | |

**User's choice:** Wrap each Polar API call in a dedicated `*Client` method — easy to patch if spec differs from reality.

---

## create_training_target Params

| Option | Description | Selected |
|--------|-------------|----------|
| Simplified flat params — handler assembles | Simple top-level params; handler builds phases internally | |
| Full phases array — Claude constructs it | Tool exposes structured phases array; Claude builds it using the skill | ✓ |

**User's choice:** Full phases array — Claude constructs the phases structure.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Optional — skill teaches defaults, tool accepts any shape | warmup/cooldown optional; SKILL.md specifies defaults | ✓ |
| Required — tool rejects phases array without warmup and cooldown | Strict validation; always requires warmup and cooldown | |

**User's choice:** Warmup and cooldown are optional; SKILL.md teaches Claude the sensible defaults.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — support multiple repeat phases in the array | Multiple repeat blocks in one session; complex structured workouts supported | ✓ |
| No — single repeat block only for v1 | One repeat block; simpler; mixed-intensity sessions deferred to v2 | |

**User's choice:** Multiple distinct repeat blocks in one phases array are supported.

---

## Intensity Label → HR Zone Mapping

| Option | Description | Selected |
|--------|-------------|----------|
| easy→Z1, aerobic→Z2 | Clean 1:1 mapping: easy=Z1, aerobic=Z2, tempo=Z3, threshold=Z4, vo2max=Z5 | ✓ |
| easy→Z2, aerobic→Z2 | Both labels map to Z2 — overlap, probably not intended | |

**User's choice:** Clean 1:1: easy=Z1, aerobic=Z2, tempo=Z3, threshold=Z4, vo2max=Z5.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Both: label or zone number | Accepts either `label: "threshold"` or `hr_zone: 4`; labels preferred | ✓ |
| Labels only | Tool only accepts intensity labels; no zone number escape hatch | |

**User's choice:** Both label and zone number accepted; handler maps labels to zones internally.

---

## Skill File Content Focus

| Option | Description | Selected |
|--------|-------------|----------|
| Equal weight: table + worked examples | HR zone table and worked examples given equal prominence | ✓ |
| Table-first, examples as reference | Zone table primary; shorter examples | |
| Examples-first, table as reference | Worked examples primary; compact zone table | |

**User's choice:** Equal weight — HR zone/label table + 2 worked examples (5×1km threshold, marathon-plan approach).

---

| Option | Description | Selected |
|--------|-------------|----------|
| Check availability first, tell user if unavailable | Pre-flight check; tell user + link to deployment docs if server missing | ✓ |
| Attempt the tool call, let the error surface | Call tool; let MCP connection failure surface naturally | |

**User's choice:** Pre-flight server availability check; tell user + link to docs if unavailable.

---

## Claude's Discretion

- Output formatting for `list_training_targets` (table, bullets, or summary prose)
- Error message wording for unlinked accounts, not-found targets, Polar API failures
- Default warmup/cooldown duration in handler when user omits phases entirely
- Sport enumeration validation approach (pass-through to Polar vs fixed-list validation)

## Deferred Ideas

- **Pace-based intensity** (`min/km` / `min/mile`) — v2 requirement INT-01; do not implement in Phase 3
- **Power-based intensity** (watts for cycling) — v2 requirement INT-02; do not implement in Phase 3
- **Sport enumeration validation** — whether to validate sport against a fixed Polar enum; left to Claude's discretion for v1
