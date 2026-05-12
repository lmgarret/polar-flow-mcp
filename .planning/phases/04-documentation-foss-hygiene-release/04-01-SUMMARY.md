---
phase: 04-documentation-foss-hygiene-release
plan: 01
subsystem: docs
tags: [mkdocs, material, diataxis, documentation]

requires:
  - phase: 03-core-mcp-tools
    provides: "MCP tool implementations, SKILL.md, docker-compose.yml authoritative env vars"
  - phase: 01-project-foundation
    provides: "DB schema migrations, HTTP routes, config structure"
provides:
  - "mkdocs.yml at repo root — Material theme, deep orange palette, 13-page nav"
  - "docs/index.md — project overview, auth model, tech stack"
  - "docs/getting-started.md — 8-step setup walkthrough (Polar app → OAuth link → first tool call)"
  - "docs/usage.md — all 4 MCP tools, HR zone table, worked examples, skill install paths"
  - "docs/security.md — threat model, AES-256-GCM scope, fail-closed rationale, header contract, key rotation plan"
  - "docs/contributing.md — clone → make test → make lint → PR flow"
  - "docs/deployment/docker-compose.md — full walkthrough with ghcr.io/lmgarret image path"
  - "docs/deployment/auth-proxies.md — real config snippets for Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access"
  - "docs/deployment/polar-oauth-setup.md — Polar developer console walkthrough"
  - "docs/reference/env-vars.md — all 10 env vars with descriptions and generate instructions"
  - "docs/reference/mcp-tools.md — formal input/output schemas for all 4 tools"
  - "docs/reference/http-endpoints.md — all 5 routes documented"
  - "docs/reference/db-schema.md — 3 tables with WAL design and AES-256-GCM blob layout"
  - "docs/images/ with 7 placeholder PNG files"
affects: [04-02-foss-hygiene, plan-02-docs-deploy]

tech-stack:
  added: [mkdocs-material]
  patterns:
    - "Diataxis documentation structure (tutorial/how-to/reference/explanation)"
    - "Placeholder image files (.png stubs) committed alongside Markdown to satisfy mkdocs strict mode"
    - "Relative image paths (../images/) from subdirectory docs pages"

key-files:
  created:
    - mkdocs.yml
    - docs/index.md
    - docs/getting-started.md
    - docs/usage.md
    - docs/security.md
    - docs/contributing.md
    - docs/deployment/docker-compose.md
    - docs/deployment/auth-proxies.md
    - docs/deployment/polar-oauth-setup.md
    - docs/reference/env-vars.md
    - docs/reference/mcp-tools.md
    - docs/reference/http-endpoints.md
    - docs/reference/db-schema.md
    - docs/images/.gitkeep
    - docs/images/claude-tool-call.png
    - docs/images/polar-oauth-flow.png
    - docs/images/docker-compose-up.png
    - docs/images/authelia-login.png
    - docs/images/polar-app-registration.png
    - docs/images/security-model.png
    - docs/images/mcp-tool-response.png
  modified: []

key-decisions:
  - "Placeholder image PNG files must be committed (even as empty files) for mkdocs build --strict to pass — strict mode validates image links"
  - "Subdirectory docs pages (deployment/, reference/) use ../images/ relative paths for images, not images/"
  - "docs/deployment/docker-compose.md documents the correct public image ghcr.io/lmgarret/polar-flow-mcp:latest and notes the repo file still has the old path"
  - "All 5 auth proxy snippets include both Remote-User and X-Proxy-Secret injection per T-04-01-02 threat mitigation"

requirements-completed: [DOC-01, DOC-02, DOC-03, DOC-04]

duration: 45min
completed: 2026-05-12
---

# Phase 4 Plan 01: MkDocs Material Documentation Site Summary

**13-page Diátaxis documentation site with mkdocs.yml, fully written prose for all deployment scenarios, AES-256-GCM security model, and real auth proxy config snippets for 5 proxy types**

## Performance

- **Duration:** ~45 min
- **Started:** 2026-05-12T15:08:18Z
- **Completed:** 2026-05-12T15:53:00Z
- **Tasks:** 2/2
- **Files modified:** 21 created

## Accomplishments

- Complete MkDocs Material site with 13 prose pages covering the Diátaxis structure
  (tutorial, how-to, reference, explanation) — `mkdocs build --strict` passes with zero warnings
- Security documentation covering threat model, AES-256-GCM at-rest scope, fail-closed
  design rationale, header contract middleware ordering, and v2 key rotation plan
- Real configuration snippets for Authelia, Authentik, oauth2-proxy, Pomerium, and
  Cloudflare Access — each with both `Remote-User` and `X-Proxy-Secret` injection
- Formal MCP tool schemas for all 4 tools including the full phases array structure with
  intensity label mapping (easy/aerobic/tempo/threshold/vo2max → Z1–Z5)

## Task Commits

1. **Task 1: mkdocs.yml and Tier-1 docs skeleton** — `023f377` (docs)
2. **Task 2: high-prose deployment, security, and reference pages** — `64a0f30` (docs)

## Files Created/Modified

- `mkdocs.yml` — Material theme config, deep orange palette, 13-entry nav
- `docs/index.md` — Project overview with features, tech stack, auth model
- `docs/getting-started.md` — Prerequisites → Polar app → Docker → OAuth link → first tool call
- `docs/usage.md` — All 4 tools, HR zone table, 3 worked examples, both skill install paths
- `docs/security.md` — Full threat model, crypto scope, fail-closed, header contract, rotation plan
- `docs/contributing.md` — Dev setup, `make test`/`make lint`, conventional commits
- `docs/deployment/docker-compose.md` — Full walkthrough, corrected image path
- `docs/deployment/auth-proxies.md` — Real YAML/nginx/Caddy snippets for 5 proxy types
- `docs/deployment/polar-oauth-setup.md` — Polar developer console walkthrough
- `docs/reference/env-vars.md` — All 10 env vars with generate commands
- `docs/reference/mcp-tools.md` — Formal JSON input/output schemas for all 4 tools
- `docs/reference/http-endpoints.md` — All 5 routes with auth model
- `docs/reference/db-schema.md` — 3 tables, WAL dual-pool design, BLOB layout
- `docs/images/` — 7 placeholder PNG files (committed empty, satisfy mkdocs strict mode)

## Decisions Made

- **Placeholder images must be actual files**: MkDocs strict mode validates image link
  targets. Empty `.png` files are committed to satisfy the validator. Real screenshots
  are not required in v1 per D-08.
- **Subdirectory relative paths**: Pages under `docs/deployment/` and `docs/reference/`
  use `../images/` for image paths, not `images/`. This matches MkDocs' path resolution.
- **Corrected image path documented**: `docker-compose.yml` in repo still references
  `ghcr.io/lm/polar-flow-mcp:latest` (old path). `docker-compose.md` notes this and
  tells operators to use `ghcr.io/lmgarret/polar-flow-mcp:latest`.
- **Both headers in every proxy snippet**: Per threat T-04-01-02, all 5 proxy
  configurations include both identity header injection AND `X-Proxy-Secret` injection.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed image paths in subdirectory pages**
- **Found during:** Task 2 verification
- **Issue:** `mkdocs build --strict` failed with 3 warnings — deployment/ pages referenced `images/X.png` which MkDocs resolved as `deployment/images/X.png` (not found)
- **Fix:** Changed image paths in `deployment/*.md` from `images/X.png` to `../images/X.png`
- **Files modified:** `docs/deployment/auth-proxies.md`, `docs/deployment/docker-compose.md`, `docs/deployment/polar-oauth-setup.md`
- **Verification:** `mkdocs build --strict` passes with zero warnings after fix
- **Committed in:** `64a0f30` (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added placeholder images for security.md and mcp-tools.md**
- **Found during:** Task 2 verification
- **Issue:** Task 2 acceptance criteria require each authored doc page to have a placeholder image (DOC-04). `docs/security.md` and `docs/reference/mcp-tools.md` had none.
- **Fix:** Created `docs/images/security-model.png` and `docs/images/mcp-tool-response.png` (empty placeholder files); added `![...]` references in both pages
- **Files modified:** `docs/security.md`, `docs/reference/mcp-tools.md`
- **Verification:** Both pages now have placeholder image syntax; `mkdocs build --strict` passes
- **Committed in:** `64a0f30` (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 bug in path resolution, 1 missing DOC-04 compliance)
**Impact on plan:** Both fixes necessary for `mkdocs build --strict` compliance and DOC-04 requirement. No scope creep.

## Issues Encountered

None beyond the auto-fixed deviations above.

## User Setup Required

None - no external service configuration required for the documentation itself. The docs content is static Markdown. Plan 02 adds the GitHub Pages deployment workflow.

## Known Stubs

None — all 13 pages are fully written prose. Placeholder images are empty PNG files by design (DOC-04 specifies placeholder syntax, not real screenshots).

## Next Phase Readiness

- `mkdocs.yml` and complete `docs/` tree are ready for Plan 02's GitHub Pages deployment workflow
- All 13 nav entries resolve to complete prose pages
- `mkdocs build --strict` verified passing — CI integration in Plan 02 will use this same command
- Requirement DOC-05 (GitHub Pages deployment) is the next deliverable

---
*Phase: 04-documentation-foss-hygiene-release*
*Completed: 2026-05-12*
