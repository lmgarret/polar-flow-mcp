---
phase: 04-documentation-foss-hygiene-release
reviewed: 2026-05-12T00:00:00Z
depth: standard
files_reviewed: 25
files_reviewed_list:
  - CHANGELOG.md
  - cliff.toml
  - CODE_OF_CONDUCT.md
  - CONTRIBUTING.md
  - docs/contributing.md
  - docs/deployment/auth-proxies.md
  - docs/deployment/docker-compose.md
  - docs/deployment/polar-oauth-setup.md
  - docs/getting-started.md
  - docs/index.md
  - docs/reference/db-schema.md
  - docs/reference/env-vars.md
  - docs/reference/http-endpoints.md
  - docs/reference/mcp-tools.md
  - docs/security.md
  - docs/usage.md
  - .github/CODEOWNERS
  - .github/ISSUE_TEMPLATE/bug_report.yml
  - .github/ISSUE_TEMPLATE/feature_request.yml
  - .github/PULL_REQUEST_TEMPLATE.md
  - .github/workflows/docs.yml
  - .github/workflows/release.yml
  - LICENSE
  - mkdocs.yml
  - README.md
  - SECURITY.md
findings:
  critical: 4
  warning: 4
  info: 2
  total: 10
status: issues_found
---

# Phase 04: Code Review Report

**Reviewed:** 2026-05-12T00:00:00Z
**Depth:** standard
**Files Reviewed:** 25
**Status:** issues_found

## Summary

Phase 4 delivered the documentation site, FOSS hygiene files, and release automation. The content itself is thorough and well-structured. However, four blockers were found: the committed `docker-compose.yml` image path still uses the old `ghcr.io/lm/` prefix that will 404 for users; the README quickstart copies `docker-compose.yml` to the nonsensical filename `.env.yml`; the release workflow's `--strip header` flag causes `CHANGELOG.md` to permanently lose its `# Changelog` header after the first release run; and `docs/contributing.md` documents a `go install` command that does not work for golangci-lint v2.x. Four warnings cover unpinned action tags in CI, the Go Report Card badge pointing to the wrong module path, the `git push origin HEAD:main` in the release workflow being vulnerable to branch protection failures that leave releases in a partial state, and an inconsistency between the two contributing guides' install methods.

---

## Critical Issues

### CR-01: `docker-compose.yml` image path will 404 for all users

**File:** `docker-compose.yml:9`
**Issue:** The committed `docker-compose.yml` has `image: ghcr.io/lm/polar-flow-mcp:latest`. The CI/CD workflow pushes images to `ghcr.io/${{ github.repository }}`, which resolves to `ghcr.io/lmgarret/polar-flow-mcp`. Every user who runs `docker compose up -d` with the as-shipped file will get an image-not-found error. The documentation (`docs/deployment/docker-compose.md` lines 44–47) acknowledges this discrepancy as "to be fixed in Phase 4" — but Phase 4 is this phase and the file was not updated.

**Fix:**
```yaml
# docker-compose.yml line 9
image: ghcr.io/lmgarret/polar-flow-mcp:latest
```

Also update the comment on line 5 to use the correct repository URL:
```yaml
# See https://github.com/lmgarret/polar-flow-mcp/docs/deployment for setup instructions.
```

---

### CR-02: README quickstart copies to wrong filename `.env.yml`

**File:** `README.md:19`
**Issue:** The Quickstart section instructs users to run `cp docker-compose.yml .env.yml`. The filename `.env.yml` has no meaning to Docker Compose; it will not be used as a compose override file. Docker Compose reads `docker-compose.yml` (or `compose.yml`) and optionally `docker-compose.override.yml`. A user following this instruction literally ends up editing a file that Docker Compose ignores entirely, then wondering why nothing works.

**Fix:**
```bash
# Option A: edit in place (simplest for homelab users)
git clone https://github.com/lmgarret/polar-flow-mcp.git
cd polar-flow-mcp
# Edit docker-compose.yml and fill in the required variables
docker compose up -d

# Option B: if the intent was an override file, use the correct name:
cp docker-compose.yml docker-compose.override.yml
```

The README should direct users to edit `docker-compose.yml` directly, or use a proper override file. The current filename `.env.yml` must be removed.

---

### CR-03: `--strip header` in release workflow corrupts `CHANGELOG.md` after first release

**File:** `.github/workflows/release.yml:68`
**Issue:** The git-cliff invocation uses `--strip header`, which strips the changelog header (`# Changelog\n\nAll notable changes...`) from the output that git-cliff-action writes to `CHANGELOG.md`. The flag is appropriate for the GitHub Release body (the `${{ steps.git-cliff.outputs.content }}` output), but it also causes the committed `CHANGELOG.md` file to permanently lose its header section after the first tag push. Every subsequent release appends version entries to a headerless file.

**Fix:** Use separate steps for the GitHub Release body and the CHANGELOG.md commit, or pass `--strip header` only to the output consumed by the release body and generate `CHANGELOG.md` separately without the flag:

```yaml
- name: Generate CHANGELOG for release body
  id: git-cliff-release
  uses: orhun/git-cliff-action@v4
  with:
    config: cliff.toml
    args: --verbose --latest --strip all   # strip everything for release body
  env:
    GITHUB_REPO: ${{ github.repository }}

- name: Update CHANGELOG.md
  id: git-cliff-full
  uses: orhun/git-cliff-action@v4
  with:
    config: cliff.toml
    args: --verbose --unreleased  # full changelog with header intact
  env:
    GITHUB_REPO: ${{ github.repository }}

# Then commit CHANGELOG.md and use git-cliff-release.outputs.content for the release body
```

Alternatively, remove `--strip header` and accept the full header in the release body (GitHub renders it fine).

---

### CR-04: `go install` for golangci-lint v2 does not work

**File:** `docs/contributing.md:19`
**Issue:** The documentation instructs contributors to install golangci-lint v2 with:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.11
```
The golangci-lint project explicitly documents that `go install` **does not work** for v2.x because of build constraints; the installed binary may be silently broken or fail to run. Their own installation guide mandates either the curl install script or a binary download. Since `CONTRIBUTING.md` (the root file) already uses the correct curl script method, `docs/contributing.md` is inconsistent and actively wrong.

**Fix:** Replace the `go install` command in `docs/contributing.md` lines 18–22 with the same curl-based install used in `CONTRIBUTING.md`:

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b ~/go/bin v2.11.0
```

---

## Warnings

### WR-01: CI workflow actions not pinned to commit SHAs

**File:** `.github/workflows/release.yml:1-84`, `.github/workflows/docs.yml:1-41`, `.github/workflows/ci.yml:1-73`
**Issue:** All three workflows use floating version tags (`@v4`, `@v5`, `@v6`, `@v9`, `@v3`) for third-party actions. A compromised action repository (e.g., `docker/build-push-action`, `softprops/action-gh-release`, `orhun/git-cliff-action`) could push a malicious tag update that runs arbitrary code in your release pipeline with `contents: write` and `packages: write` permissions. This is a known supply-chain attack vector (similar to the `tj-actions/changed-files` incident in 2025). The release workflow is especially high-risk: it has access to `GITHUB_TOKEN` with both `packages: write` (can publish images) and `contents: write` (can push to main).

**Fix:** Pin all third-party actions to their full commit SHA. Example for `docker/build-push-action@v6`:
```yaml
# Find the SHA: gh api repos/docker/build-push-action/git/ref/tags/v6 --jq .object.sha
uses: docker/build-push-action@263435318d21b8e681c14492fe198d362a7d2c83  # v6.18.0
```

At minimum, pin actions in `release.yml` since it has write permissions to packages and contents.

---

### WR-02: Go Report Card badge points to wrong module path

**File:** `README.md:9`
**Issue:** The badge URL is `https://goreportcard.com/badge/github.com/lmgarret/polar-flow-mcp`, but `go.mod` declares the module as `github.com/lm/polar-flow-mcp`. Go Report Card indexes modules by their declared module path, not by the GitHub repository URL. The badge will either fail to resolve or display a report for a non-existent module.

**Fix:** Either update the badge to reference the actual module path:
```markdown
[![Go Report Card](https://goreportcard.com/badge/github.com/lm/polar-flow-mcp)](https://goreportcard.com/report/github.com/lm/polar-flow-mcp)
```
Or — if the module path is being updated to `github.com/lmgarret/polar-flow-mcp` as part of this phase — update `go.mod` and all internal import paths accordingly and use the `lmgarret` path in the badge.

---

### WR-03: Release workflow `git push origin HEAD:main` fails silently under branch protection, leaving partial release

**File:** `.github/workflows/release.yml:78`
**Issue:** The release workflow pushes the CHANGELOG commit directly to `main` using `git push origin HEAD:main`. If branch protection rules require pull requests or status checks on `main`, this push will fail. At that point the Docker image has already been published (step 54) and the GitHub Release will not be created (step 80 is skipped because step 72 failed). The result is a published Docker image with no corresponding GitHub Release and no CHANGELOG update — a partially-complete release that requires manual cleanup.

**Fix:** Add `continue-on-error: true` to the CHANGELOG commit step and document the manual fallback, or — preferably — configure a branch protection exception for the `github-actions[bot]` actor. If the repo does not use branch protection, add a comment noting this assumption:

```yaml
- name: Commit CHANGELOG.md
  continue-on-error: true   # graceful degradation if branch protection is in effect
  run: |
    git config user.name github-actions[bot]
    git config user.email 41898282+github-actions[bot]@users.noreply.github.com
    git add CHANGELOG.md
    git diff --staged --quiet || git commit -m "chore(release): update CHANGELOG.md for ${{ github.ref_name }} [skip ci]"
    git push origin HEAD:main
```

---

### WR-04: `docs/contributing.md` golangci-lint install method inconsistent with `CONTRIBUTING.md`

**File:** `docs/contributing.md:19`
**Issue:** Beyond the functional breakage documented in CR-04, the two contributing guides give different install methods for the same tool and version. `CONTRIBUTING.md` (root) uses the official `curl` install script. `docs/contributing.md` uses `go install`. There is one authoritative method; the other should be removed. When contributors follow the MkDocs site (the more discoverable path), they hit a broken install command.

**Fix:** This is resolved by the CR-04 fix. Once both files use the same `curl` install method, delete the `go install` block from `docs/contributing.md` entirely and keep only the curl form. Consider having `docs/contributing.md` state that `CONTRIBUTING.md` is the authoritative source (it already does at line 6) and reduce duplication.

---

## Info

### IN-01: `docker-compose.yml` comment URL still uses `github.com/lm/` path

**File:** `docker-compose.yml:5`
**Issue:** The comment in the committed `docker-compose.yml` reads:
```
# See https://github.com/lm/polar-flow-mcp/docs/deployment for setup instructions.
```
The correct GitHub repository is `lmgarret/polar-flow-mcp`. This URL will 404 for users reading the comment. This is partially addressed by CR-01's fix but worth noting as its own line item since the comment and the `image:` field are separate.

**Fix:** Update the comment URL:
```yaml
# See https://github.com/lmgarret/polar-flow-mcp/docs/deployment for setup instructions.
```

---

### IN-02: `docs/deployment/docker-compose.md` documents the wrong image path as a known deferral

**File:** `docs/deployment/docker-compose.md:44-47`
**Issue:** Lines 44–47 contain a note acknowledging that the committed `docker-compose.yml` uses the wrong image path and states it "will not be updated until Phase 4 release." Phase 4 is this phase. Once CR-01 is fixed, this note should be removed — it will be confusing and incorrect after the fix.

**Fix:** After applying the CR-01 fix, remove the `> **Note:**` block at lines 44–47 of `docs/deployment/docker-compose.md` so the documentation no longer describes a known-broken file.

---

_Reviewed: 2026-05-12T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
