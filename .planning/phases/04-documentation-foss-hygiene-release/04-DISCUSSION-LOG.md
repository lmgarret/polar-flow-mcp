# Phase 4: Documentation, FOSS Hygiene, Release - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-11
**Phase:** 4-Documentation, FOSS Hygiene, Release
**Areas discussed:** GitHub repo identity, Release Docker build targets, Docs content depth, git-cliff changelog style

---

## GitHub Repo Identity

| Option | Description | Selected |
|--------|-------------|----------|
| github.com/lm/polar-flow-mcp (go.mod path) | Use the go.mod module path as-is for all GitHub-facing output | |
| lmgarret/polar-flow-mcp | Different org than go.mod — the real GitHub remote | ✓ |

**User's choice:** `lmgarret/polar-flow-mcp`
**Notes:** The Go module path (`github.com/lm/polar-flow-mcp`) does not change — it is internal only. All GitHub-facing items (badges, GHCR, Pages URL) use `lmgarret/polar-flow-mcp`.

---

## Release Docker Build Targets

### Architecture

| Option | Description | Selected |
|--------|-------------|----------|
| amd64 + arm64 | Multi-arch via QEMU/buildx; covers homelab RPi/NAS/Apple Silicon | ✓ |
| amd64-only | Simpler release.yml; arm64 deferred to v2 | |

**User's choice:** amd64 + arm64

### Latest tag on release

| Option | Description | Selected |
|--------|-------------|----------|
| latest + semver tag | ghcr.io/:1.0.0 and :latest both updated on tag push | ✓ |
| Semver tag only | Only ghcr.io/:1.0.0; no floating latest | |

**User's choice:** latest + semver tag

---

## Docs Content Depth

### Content completeness

| Option | Description | Selected |
|--------|-------------|----------|
| Fully written prose | Each Diátaxis page deployable from docs alone | ✓ |
| Skeleton with key paragraphs | Structure correct, enough for reviewers | |
| Structure only | Page stubs with placeholder content markers | |

**User's choice:** Fully written prose

### Versioning strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Latest-always, no versioning | docs.yml pushes to gh-pages on every main push; single version | ✓ |
| mike versioning from start | Multiple version docs; add complexity now | |

**User's choice:** Latest-always, no versioning

---

## git-cliff Changelog Style

### CHANGELOG location

| Option | Description | Selected |
|--------|-------------|----------|
| Repo root (CHANGELOG.md) | Committed to repo; visible on GitHub; generated on release | ✓ |
| GitHub Releases body only | No file in repo; only in Release description | |

**User's choice:** Repo root

### Commit grouping

| Option | Description | Selected |
|--------|-------------|----------|
| Standard groups (feat/fix/docs/chore) | Matches project's existing commit conventions | ✓ |
| Claude's discretion | Let planner pick cliff.toml config | |

**User's choice:** Standard groups (feat → Features, fix → Bug Fixes, docs → Documentation, chore → Maintenance)

---

## Claude's Discretion

- Exact `cliff.toml` format and additional commit type handling (test, refactor, ci)
- README badge order and exact quickstart wording (must be 5 lines, docker-compose path)
- `mkdocs.yml` color theme and nav structure beyond the required pages
- Whether to include `CODEOWNERS`
- Exact issue template fields

## Deferred Ideas

None — discussion stayed within phase scope.
