---
status: partial
phase: 04-documentation-foss-hygiene-release
source: [04-VERIFICATION.md]
started: 2026-05-12T00:00:00Z
updated: 2026-05-12T00:00:00Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. Release workflow end-to-end
expected: Push a `v1.0.0` tag; multi-arch Docker image appears at `ghcr.io/lmgarret/polar-flow-mcp:1.0.0` and `:latest`; GitHub Release is created with changelog body; `CHANGELOG.md` is updated on `main` with its header intact; `docs.yml` is NOT re-triggered by the CHANGELOG commit (due to `paths-ignore: CHANGELOG.md`)
result: [pending]

### 2. GitHub Pages deployment
expected: Push to `main`; `https://lmgarret.github.io/polar-flow-mcp` renders the full MkDocs Material site with all nav links working (Getting Started, Deployment, Usage, Reference, Security, Contributing)
result: [pending]

### 3. README badge rendering
expected: Open the repo on GitHub after at least one CI run; all 5 badges (CI status, license, latest release, image size, Go Report Card) resolve and display correctly
result: [pending]

### 4. Contributor onboarding
expected: Follow `CONTRIBUTING.md` from a clean clone on a machine without golangci-lint; the curl install script works, `~/go/bin/golangci-lint --version` reports v2.11.x, `make test` passes
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
