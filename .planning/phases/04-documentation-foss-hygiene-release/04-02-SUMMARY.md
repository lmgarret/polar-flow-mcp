---
phase: 04
plan: 02
subsystem: ci
tags: [ci, github-actions, release, git-cliff, mkdocs-deploy]
dependency_graph:
  requires: []
  provides: [docs-deploy-workflow, release-workflow, cliff-config, changelog-stub]
  affects: [.github/workflows/]
tech_stack:
  added: [orhun/git-cliff-action@v4, softprops/action-gh-release@v3, docker/setup-qemu-action@v3, docker/setup-buildx-action@v3, actions/setup-python@v5, actions/cache@v4]
  patterns: [multi-arch-docker-build, mkdocs-gh-deploy, git-cliff-changelog]
key_files:
  created:
    - .github/workflows/docs.yml
    - .github/workflows/release.yml
    - cliff.toml
    - CHANGELOG.md
  modified: []
decisions:
  - "Release body uses steps.git-cliff.outputs.content (not body_path: CHANGELOG.md) — current section only"
  - "docs.yml ignores CHANGELOG.md via paths-ignore AND release uses [skip ci] — defense in depth for T-04-02-04"
  - "QEMU step precedes Buildx step precedes build-push — mandatory ordering for linux/arm64"
metrics:
  duration: "1 minute"
  completed_date: "2026-05-12"
  tasks_completed: 3
  files_created: 4
---

# Phase 4 Plan 2: CI Workflows (docs.yml + release.yml + cliff.toml) Summary

Two new GitHub Actions workflows, a git-cliff config, and a CHANGELOG stub wired for DOC-05 (GitHub Pages deploy) and FOSS-03 (release automation with multi-arch Docker + git-cliff CHANGELOG).

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Create cliff.toml and CHANGELOG.md stub | c0030be | cliff.toml, CHANGELOG.md |
| 2 | Create docs.yml workflow (GitHub Pages deploy) | d8c60df | .github/workflows/docs.yml |
| 3 | Create release.yml workflow (multi-arch Docker + GitHub Release) | ef690e6 | .github/workflows/release.yml |

## What Was Built

**cliff.toml** — git-cliff configuration with D-10 commit groups (Features, Bug Fixes, Documentation, Maintenance), `chore(release)` skip rule to prevent self-referential CHANGELOG entries, `trim_start_matches(pat="v")` for version display stripping, and `tag_pattern = "v[0-9].*"` for proper tag matching.

**CHANGELOG.md** — Header-only stub so the first `release.yml` run finds an existing file to overwrite rather than encountering a missing-file edge case.

**docs.yml** — Deploys MkDocs Material site to GitHub Pages on every push to `main`. Uses `paths-ignore: ['CHANGELOG.md']` to prevent re-triggering from the release bot's CHANGELOG commit. Python 3.x + pip cache with weekly cache key. No Go toolchain (MkDocs builds Markdown only). Bot identity configured for `mkdocs gh-deploy`'s internal git operations.

**release.yml** — Tag-triggered (`v*`) release pipeline in a single job:
1. Test gate: `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...` + golangci-lint v2.11
2. Multi-arch Docker: QEMU → Buildx → metadata-action (semver strips `v`, + latest) → build-push (amd64 + arm64)
3. Changelog: git-cliff-action@v4 with `--latest --strip header` generates current-version section
4. CHANGELOG commit: bot commit with `[skip ci]` pushed to main
5. GitHub Release: softprops@v3 with `body: steps.git-cliff.outputs.content` (not cumulative `body_path:`)

## Decisions Made

- **Release body wiring:** `steps.git-cliff.outputs.content` not `body_path: CHANGELOG.md` — the former contains only the current tag's section; the latter is the entire cumulative changelog.
- **Double re-trigger defense (T-04-02-04):** Both `[skip ci]` in the CHANGELOG commit message AND `paths-ignore: ['CHANGELOG.md']` in docs.yml — either alone is a single point of failure.
- **Step ordering non-negotiable:** QEMU (line 40) → Buildx (43) → build-push (55) → git-cliff (65) → gh-release (81). Docker push completes before any git/release operations that could fail.
- **No PATs:** All workflows use `GITHUB_TOKEN` with minimum required scope (`contents: write` + `packages: write` for release; `contents: write` only for docs).

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None — all files are complete and functional. The CHANGELOG.md stub is intentional by plan design (git-cliff overwrites on first release).

## Threat Surface Scan

No new threat surface beyond the plan's threat model. All T-04-02-* mitigations applied:
- T-04-02-01: permissions scoped to minimum required (`contents: write` + `packages: write`)
- T-04-02-04: double defense applied (paths-ignore + [skip ci])
- T-04-02-06: `type=semver,pattern={{version}}` used (not `type=ref,event=tag`)
- T-04-02-07: `fetch-depth: 0` applied to release.yml checkout

## Self-Check: PASSED

- cliff.toml exists: FOUND
- CHANGELOG.md exists: FOUND
- .github/workflows/docs.yml exists: FOUND
- .github/workflows/release.yml exists: FOUND
- Commit c0030be: FOUND (cliff.toml + CHANGELOG.md)
- Commit d8c60df: FOUND (docs.yml)
- Commit ef690e6: FOUND (release.yml)
