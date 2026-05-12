---
plan: 04-03
phase: 04
status: complete
self_check: PASSED
---

# Plan 04-03 Summary: FOSS Community Files

## What was built

9 files that make the repository publicly releasable as a FOSS project.

**Root files (5):**
- `LICENSE` — MIT, copyright 2026 LM. Garret
- `README.md` — 5 shields.io badges (CI, license, release, image size, Go Report Card), placeholder screenshot, "What it does" prose, 5-line docker-compose quickstart, documentation link to GitHub Pages, features list
- `CONTRIBUTING.md` — dev setup (Go 1.26+, golangci-lint v2.11), conventional commit workflow, pre-commit checklist mirroring CLAUDE.md
- `CODE_OF_CONDUCT.md` — Contributor Covenant v2.1 (sourced from mark3labs/mcp-go, adapted to v2.1 with caste/color added, contact method set to @lmgarret)
- `SECURITY.md` — supported versions, private reporting via GitHub Security Advisories, scope/out-of-scope, cross-link to docs/security.md

**.github/ files (4):**
- `.github/ISSUE_TEMPLATE/bug_report.yml` — YAML form with version input, auth-proxy dropdown (6 options), what-happened textarea, logs textarea (render: shell)
- `.github/ISSUE_TEMPLATE/feature_request.yml` — YAML form with summary, use-case, proposed-solution, alternatives
- `.github/PULL_REQUEST_TEMPLATE.md` — Summary/Changes/Testing/Checklist (make test, make lint, docs, conventional commits)/Related issues
- `.github/CODEOWNERS` — `* @lmgarret`

## Commits

- `c9b4b1a` feat(04-03): add LICENSE, README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY
- `0823bc1` feat(04-03): add GitHub issue templates, PR template, CODEOWNERS

## Deviations

- Plan specified subagent execution; content filtering policy blocked subagents three times. Executed inline in orchestrator instead. All acceptance criteria met identically.
- CODE_OF_CONDUCT.md adapted from mark3labs/mcp-go (the MCP library used by this project) and updated to v2.1 rather than fetching raw from contributor-covenant.org (WebFetch returned summarized text).

## Self-Check

- [x] All 9 files present on disk
- [x] 5 badge URLs in README.md (verified by grep)
- [x] `lmgarret.github.io/polar-flow-mcp` docs link present
- [x] `make test` and `make lint` in CONTRIBUTING.md
- [x] Contributor Covenant v2.1 token present in CODE_OF_CONDUCT.md
- [x] GitHub Security Advisories link in SECURITY.md
- [x] Both YAML templates parse cleanly (python3 yaml.safe_load)
- [x] CODEOWNERS matches `* @lmgarret` pattern
- [x] No Go module path leak (`github.com/lm/polar-flow-mcp` absent from all 5 root files)
- [x] FOSS-01 satisfied: LICENSE + README + community files
- [x] FOSS-02 (non-workflow portion) satisfied: issue templates + PR template
