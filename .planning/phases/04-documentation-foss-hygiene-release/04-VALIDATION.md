---
phase: 4
slug: documentation-foss-hygiene-release
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-11
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + shell/CI script checks |
| **Config file** | none — documentation and workflow phase |
| **Quick run command** | `mkdocs build --strict 2>&1 | tail -5` |
| **Full suite command** | `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./... && mkdocs build --strict` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `mkdocs build --strict 2>&1 | tail -5`
- **After every plan wave:** Run `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./... && mkdocs build --strict`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 4-01-01 | 01 | 1 | DOC-01 | — | N/A | file-check | `test -f docs/index.md && test -f mkdocs.yml` | ❌ W0 | ⬜ pending |
| 4-01-02 | 01 | 1 | DOC-02 | — | N/A | file-check | `test -f docs/getting-started.md && test -f docs/deployment/auth-proxies.md` | ❌ W0 | ⬜ pending |
| 4-01-03 | 01 | 1 | DOC-03 | — | N/A | file-check | `test -f docs/security.md` | ❌ W0 | ⬜ pending |
| 4-01-04 | 01 | 1 | DOC-04 | — | N/A | build | `mkdocs build --strict 2>&1 | grep -c ERROR | grep -q ^0$` | ❌ W0 | ⬜ pending |
| 4-02-01 | 02 | 2 | DOC-05 | — | N/A | file-check | `test -f .github/workflows/docs.yml && test -f .github/workflows/release.yml` | ❌ W0 | ⬜ pending |
| 4-02-02 | 02 | 2 | FOSS-02 | — | N/A | file-check | `test -f cliff.toml` | ❌ W0 | ⬜ pending |
| 4-02-03 | 02 | 2 | FOSS-03 | — | N/A | syntax | `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml'))"` | ❌ W0 | ⬜ pending |
| 4-03-01 | 03 | 2 | FOSS-01 | — | N/A | file-check | `test -f LICENSE && test -f README.md && test -f CONTRIBUTING.md` | ❌ W0 | ⬜ pending |
| 4-03-02 | 03 | 2 | FOSS-01 | — | N/A | file-check | `test -f CODE_OF_CONDUCT.md && test -f SECURITY.md` | ❌ W0 | ⬜ pending |
| 4-03-03 | 03 | 2 | FOSS-02 | — | N/A | file-check | `test -f .github/PULL_REQUEST_TEMPLATE.md && ls .github/ISSUE_TEMPLATE/*.yml 2>/dev/null | wc -l | grep -qv ^0$` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `mkdocs` and `mkdocs-material` installed in environment (for build checks)
- [ ] `python3` with `yaml` module available (for YAML syntax checks)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| GitHub Pages renders correctly | DOC-05 | Requires live Pages deployment | Push to main, visit https://lm.github.io/polar-flow-mcp |
| Release workflow creates GitHub Release | FOSS-03 | Requires pushing a v* tag | Push v0.0.1-test tag, verify Release appears in GitHub UI |
| Multi-arch image available at ghcr.io | FOSS-03 | Requires registry push | `docker manifest inspect ghcr.io/lm/polar-flow-mcp:latest` |
| README badges all resolve | FOSS-01 | Live HTTP checks | Open README in browser, verify all badges load |
| New contributor can follow CONTRIBUTING.md | FOSS-01 | Human judgment | Follow guide from scratch on clean clone |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
