# Phase 4: Documentation, FOSS Hygiene, Release - Pattern Map

**Mapped:** 2026-05-11
**Files analyzed:** 26 new/modified files
**Analogs found:** 8 / 26 (remaining 18 are pure prose with no code analog in this repo)

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `.github/workflows/release.yml` | config/CI | event-driven (tag push) | `.github/workflows/ci.yml` | role-match |
| `.github/workflows/docs.yml` | config/CI | event-driven (branch push) | `.github/workflows/ci.yml` | role-match |
| `mkdocs.yml` | config | transform | RESEARCH.md Pattern 3 | no analog |
| `cliff.toml` | config | transform | RESEARCH.md Pattern 4 | no analog |
| `CHANGELOG.md` | config | batch (generated) | none | no analog |
| `LICENSE` | config | none | none | no analog |
| `README.md` | config | none | `docker-compose.yml` (env var docs) | partial |
| `CONTRIBUTING.md` | config | none | `Makefile` (target names referenced) | partial |
| `CODE_OF_CONDUCT.md` | config | none | none | no analog |
| `SECURITY.md` | config | none | none | no analog |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | config | none | RESEARCH.md Pattern 6 | no analog |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | config | none | RESEARCH.md Pattern 6 | no analog |
| `.github/PULL_REQUEST_TEMPLATE.md` | config | none | none | no analog |
| `.github/CODEOWNERS` | config | none | none | no analog |
| `docs/index.md` | config/doc | none | none | no analog |
| `docs/getting-started.md` | config/doc | none | none | no analog |
| `docs/usage.md` | config/doc | none | `skill/polar-coach/SKILL.md` | partial |
| `docs/security.md` | config/doc | none | none | no analog |
| `docs/contributing.md` | config/doc | none | `Makefile` | partial |
| `docs/deployment/docker-compose.md` | config/doc | none | `docker-compose.yml` | partial |
| `docs/deployment/auth-proxies.md` | config/doc | none | none | no analog |
| `docs/deployment/polar-oauth-setup.md` | config/doc | none | none | no analog |
| `docs/reference/env-vars.md` | config/doc | none | `docker-compose.yml` (env section) | partial |
| `docs/reference/mcp-tools.md` | config/doc | none | `skill/polar-coach/SKILL.md` | partial |
| `docs/reference/http-endpoints.md` | config/doc | none | none | no analog |
| `docs/reference/db-schema.md` | config/doc | none | none | no analog |

---

## Pattern Assignments

### `.github/workflows/release.yml` (config/CI, event-driven)

**Analog:** `.github/workflows/ci.yml`

**Trigger + permissions pattern** — mirror from ci.yml but change trigger and add packages write:
```yaml
# ci.yml lines 1-8 (trigger pattern to mirror):
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

# release.yml replaces the above with:
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: write
  packages: write
```

**Go setup pattern** (ci.yml lines 16-19) — copy verbatim into release.yml test gate:
```yaml
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
```

**Test step** (ci.yml lines 21-22) — copy verbatim:
```yaml
      - name: Run tests
        run: CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...
```

**Lint step** (ci.yml lines 35-38) — copy verbatim:
```yaml
      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11
```

**GHCR login pattern** (ci.yml lines 50-55) — copy verbatim:
```yaml
      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
```

**Docker metadata pattern** (ci.yml lines 57-64) — use in release.yml but change tags block to semver + latest per D-04:
```yaml
# ci.yml original (lines 57-64):
      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix=sha-,format=short

# release.yml replacement (D-04):
      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=raw,value=latest
```

**Build and push pattern** (ci.yml lines 66-72) — extend with multi-arch platforms per D-03; add QEMU and Buildx steps before:
```yaml
# Steps to insert BEFORE build-push (D-03):
      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

# Modified build step (add platforms:, remove conditional push:):
      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

**checkout pattern** — release.yml requires `fetch-depth: 0` (ci.yml uses default depth 1, which is insufficient for git-cliff):
```yaml
# release.yml checkout (NOT the ci.yml default):
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # REQUIRED: git-cliff needs full tag history
```

**git-cliff + CHANGELOG commit + GitHub Release pattern** (no analog in codebase; from RESEARCH.md):
```yaml
      - name: Generate CHANGELOG
        id: git-cliff
        uses: orhun/git-cliff-action@v4
        with:
          config: cliff.toml
          args: --verbose --latest --strip header
        env:
          GITHUB_REPO: ${{ github.repository }}

      - name: Commit CHANGELOG.md
        run: |
          git config user.name github-actions[bot]
          git config user.email 41898282+github-actions[bot]@users.noreply.github.com
          git add CHANGELOG.md
          git diff --staged --quiet || git commit -m "chore(release): update CHANGELOG.md for ${{ github.ref_name }} [skip ci]"
          git push origin HEAD:main

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v3
        with:
          body: ${{ steps.git-cliff.outputs.content }}
          make_latest: true
```

**Critical:** `[skip ci]` in the CHANGELOG commit message prevents `docs.yml` from re-triggering on the bot commit. `steps.git-cliff.outputs.content` (not `body_path: CHANGELOG.md`) is the correct release body — it contains only the current tag section.

---

### `.github/workflows/docs.yml` (config/CI, event-driven)

**Analog:** `.github/workflows/ci.yml`

**Trigger + permissions pattern** — push to main only, write for gh-pages deploy:
```yaml
name: Docs

on:
  push:
    branches: [main]
    paths-ignore:
      - 'CHANGELOG.md'   # prevent double-trigger from release.yml bot commit

permissions:
  contents: write
```

**Checkout pattern** — include fetch-depth 0 for future git-revision-date plugin compatibility:
```yaml
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
```

**Git credentials for gh-deploy** — same bot identity as release.yml CHANGELOG commit:
```yaml
      - name: Configure Git credentials
        run: |
          git config user.name github-actions[bot]
          git config user.email 41898282+github-actions[bot]@users.noreply.github.com
```

**Python + pip cache + mkdocs deploy** (no Go analog; from RESEARCH.md Pattern 1):
```yaml
      - uses: actions/setup-python@v5
        with:
          python-version: 3.x

      - run: echo "cache_id=$(date --utc '+%V')" >> $GITHUB_ENV

      - uses: actions/cache@v4
        with:
          key: mkdocs-material-${{ env.cache_id }}
          path: ~/.cache
          restore-keys: |
            mkdocs-material-

      - run: pip install mkdocs-material

      - run: mkdocs gh-deploy --force
```

**No Go toolchain needed in docs.yml** — MkDocs builds Markdown only; do not copy the `actions/setup-go@v5` step from ci.yml into this workflow.

---

### `mkdocs.yml` (config, transform)

**Analog:** RESEARCH.md Pattern 3 (no codebase analog exists)

**Full pattern from RESEARCH.md** — site identity uses D-01 hardcoded values:
```yaml
site_name: polar-flow-mcp
site_url: https://lmgarret.github.io/polar-flow-mcp
site_description: Multi-user MCP server wrapping the Polar AccessLink API
repo_url: https://github.com/lmgarret/polar-flow-mcp
repo_name: lmgarret/polar-flow-mcp

theme:
  name: material
  palette:
    primary: deep orange
    accent: orange
  features:
    - navigation.tabs
    - navigation.sections
    - navigation.expand
    - toc.integrate
    - search.suggest

plugins:
  - search

nav:
  - Home: index.md
  - Getting Started: getting-started.md
  - Deployment:
    - Docker Compose: deployment/docker-compose.md
    - Auth Proxies: deployment/auth-proxies.md
    - Polar OAuth Setup: deployment/polar-oauth-setup.md
  - Usage: usage.md
  - Reference:
    - Environment Variables: reference/env-vars.md
    - MCP Tools: reference/mcp-tools.md
    - HTTP Endpoints: reference/http-endpoints.md
    - Database Schema: reference/db-schema.md
  - Security: security.md
  - Contributing: contributing.md
```

**Do not add:** `social` plugin (requires `cairosvg` + `pillow`), `mike` (D-07 forbids versioned docs), auto-detected nav (ordering is alphabetical, breaks Diátaxis intent).

---

### `cliff.toml` (config, transform)

**Analog:** RESEARCH.md Pattern 4 (no codebase analog exists)

**Full pattern from RESEARCH.md** — includes `chore(release)` skip to prevent the CHANGELOG bot commit from re-appearing in the next CHANGELOG:
```toml
[remote.github]
owner = "lmgarret"
repo = "polar-flow-mcp"

[changelog]
header = """
# Changelog

All notable changes to this project will be documented in this file.
"""

body = """
{% if version -%}
## [{{ version | trim_start_matches(pat="v") }}] - {{ timestamp | date(format="%Y-%m-%d") }}
{% else -%}
## [Unreleased]
{% endif -%}
{% for group, commits in commits | group_by(attribute="group") %}
### {{ group | upper_first }}
{% for commit in commits %}
- {{ commit.message | split(pat="\\n") | first | upper_first | trim }}\
{% endfor %}
{% endfor %}
"""

trim = true

[git]
conventional_commits = true
filter_unconventional = true
split_commits = false
commit_parsers = [
    { message = "^feat", group = "Features" },
    { message = "^fix", group = "Bug Fixes" },
    { message = "^docs", group = "Documentation" },
    { message = "^chore\\(release\\)", skip = true },
    { message = "^chore", group = "Maintenance" },
    { message = "^refactor", group = "Refactor" },
    { message = "^test", skip = true },
    { message = "^ci", skip = true },
    { message = "^style", skip = true },
]
tag_pattern = "v[0-9].*"
```

**D-10 compliance:** `trim_start_matches(pat="v")` strips the `v` prefix in version display (v1.0.0 → 1.0.0 in CHANGELOG). `chore(release)` skip prevents the bot CHANGELOG commit from appearing in the next release's changelog.

---

### `README.md` (config/doc)

**Analog:** `docker-compose.yml` (env var documentation style) + RESEARCH.md Pattern 5

**Badge block pattern** (RESEARCH.md Pattern 5 — hardcoded to D-01 identity):
```markdown
[![CI](https://img.shields.io/github/actions/workflow/status/lmgarret/polar-flow-mcp/ci.yml?branch=main&label=CI)](https://github.com/lmgarret/polar-flow-mcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/lmgarret/polar-flow-mcp)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/lmgarret/polar-flow-mcp)](https://github.com/lmgarret/polar-flow-mcp/releases/latest)
[![Image Size](https://ghcr-badge.egpl.dev/lmgarret/polar-flow-mcp/size)](https://github.com/lmgarret/polar-flow-mcp/pkgs/container/polar-flow-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/lmgarret/polar-flow-mcp)](https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp)
```

**Go Report Card URL uses GitHub repo path** (`github.com/lmgarret/polar-flow-mcp`), NOT the Go module path (`github.com/lm/polar-flow-mcp`). These are different — see D-01 vs D-02.

**5-line quickstart pattern** (CONTEXT.md specifics):
```markdown
## Quickstart

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cp docker-compose.yml .env.yml  # copy and edit env vars
# Set ENCRYPTION_KEY, PROXY_SHARED_SECRET, POLAR_CLIENT_ID, POLAR_CLIENT_SECRET
docker compose up -d
# Visit https://lmgarret.github.io/polar-flow-mcp for full setup docs
```
```

**Env var documentation style** — mirror the comment style from `docker-compose.yml` lines 12-38 (REQUIRED/OPTIONAL labels, generate instructions for secrets, link to docs URL).

---

### `CONTRIBUTING.md` (config/doc)

**Analog:** `Makefile` (target names and commands)

**Makefile targets to reference** (Makefile lines 1-13):
```makefile
build:   CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/polar-flow-mcp ./cmd/polar-flow-mcp
test:    CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...
lint:    ~/go/bin/golangci-lint run ./...
clean:   rm -rf bin/
```

CONTRIBUTING.md must reference `make test` and `make lint` as the pre-PR checklist commands. The lint binary path (`~/go/bin/golangci-lint`) and version (`v2.11`) come from `CLAUDE.md`.

---

### `.github/ISSUE_TEMPLATE/bug_report.yml` (config)

**Analog:** RESEARCH.md Pattern 6 (YAML issue template format)

**Structural pattern** (RESEARCH.md Pattern 6) — adapt for this project's context (Docker image tag, auth proxy type):
```yaml
name: Bug Report
description: File a bug report
labels: [bug]
body:
  - type: markdown
    attributes:
      value: |
        Thanks for taking the time to fill out this bug report!
  - type: input
    id: version
    attributes:
      label: Version
      description: Which Docker image tag or git SHA are you running?
      placeholder: "ghcr.io/lmgarret/polar-flow-mcp:1.0.0"
    validations:
      required: true
  - type: dropdown
    id: auth-proxy
    attributes:
      label: Auth proxy
      options: [Authelia, Authentik, oauth2-proxy, Pomerium, "Cloudflare Access", Other]
    validations:
      required: true
  - type: textarea
    id: what-happened
    attributes:
      label: What happened?
      description: What did you expect to happen?
    validations:
      required: true
  - type: textarea
    id: logs
    attributes:
      label: Relevant log output
      render: shell
```

---

### `docs/deployment/docker-compose.md` (doc)

**Analog:** `docker-compose.yml` (the file being documented)

**Content source** — `docker-compose.yml` lines 1-41 are the primary reference. The doc page is a walkthrough of every env var with explanation of why it is required, how to generate secrets, and what the fail-closed behavior means (AUTH_PROXY=unconfigured causes server to refuse to start).

**Image reference in docs** — note that `docker-compose.yml` line 9 uses `ghcr.io/lm/polar-flow-mcp:latest` (old module path). Documentation should reference the correct public image: `ghcr.io/lmgarret/polar-flow-mcp:latest`.

---

### `docs/usage.md` + `docs/reference/mcp-tools.md` (doc)

**Analog:** `skill/polar-coach/SKILL.md`

The SKILL.md file (9,912 bytes) documents all MCP tools from a user perspective. `usage.md` and `mcp-tools.md` should draw from the same tool descriptions, input schemas, and example invocations defined there. Read `skill/polar-coach/SKILL.md` during implementation for the authoritative tool list and parameter descriptions.

---

### `docs/reference/env-vars.md` (doc)

**Analog:** `docker-compose.yml` lines 12-38

All env var names, REQUIRED/OPTIONAL status, defaults, and generation commands are already documented in `docker-compose.yml` comments. `env-vars.md` expands those inline comments into full prose paragraphs with examples.

---

## Shared Patterns

### GitHub Identity (apply to all new files with any URL or image reference)

**Source:** CONTEXT.md Decisions D-01 / D-02

| Identifier | Value |
|------------|-------|
| GitHub org/repo | `lmgarret/polar-flow-mcp` |
| GHCR image | `ghcr.io/lmgarret/polar-flow-mcp` |
| GitHub Pages | `https://lmgarret.github.io/polar-flow-mcp` |
| Go module (internal only) | `github.com/lm/polar-flow-mcp` |
| Go Report Card URL | `https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp` |

**Apply to:** README.md (badges), mkdocs.yml (site_url, repo_url), cliff.toml (remote.github), docs/*.md (any cross-links), release.yml (hardcoded references).

### Action Version Pins (apply to all new workflows)

**Source:** `.github/workflows/ci.yml` (all lines) — do not deviate from these versions

| Action | Version | Copy from ci.yml |
|--------|---------|------------------|
| `actions/checkout` | `@v4` | line 14 / 27 / 46 |
| `actions/setup-go` | `@v5` | line 16 / 28 |
| `golangci/golangci-lint-action` | `@v9` with `version: v2.11` | lines 36-38 |
| `docker/login-action` | `@v3` | line 50 |
| `docker/metadata-action` | `@v5` | line 57 |
| `docker/build-push-action` | `@v6` | line 65 |

New actions not in ci.yml (from RESEARCH.md — locked versions):
| Action | Version |
|--------|---------|
| `actions/setup-python` | `@v5` |
| `actions/cache` | `@v4` |
| `docker/setup-qemu-action` | `@v3` |
| `docker/setup-buildx-action` | `@v3` |
| `orhun/git-cliff-action` | `@v4` |
| `softprops/action-gh-release` | `@v3` |

### Go Test Command (apply to release.yml test gate)

**Source:** `ci.yml` line 22 + `Makefile` line 7

```
CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...
```

Must be copied verbatim — the `-tags=polartest` flag is required for the test suite (defined in `.golangci.yml` `run.build-tags`).

### GITHUB_TOKEN Scope (apply to all new workflows)

**Source:** ci.yml lines 43-45 (docker job permissions block)

Use `GITHUB_TOKEN` only — no PATs. Minimum required scope per workflow:
- `docs.yml`: `permissions: contents: write` (for gh-pages push)
- `release.yml`: `permissions: contents: write` + `packages: write` (for CHANGELOG commit + GHCR push + GitHub Release)

### Git Bot Identity (apply anywhere a workflow commits to the repo)

**Source:** RESEARCH.md Pattern 1 and Pattern 2 (consistent across both workflows)

```yaml
git config user.name github-actions[bot]
git config user.email 41898282+github-actions[bot]@users.noreply.github.com
```

Apply to: `docs.yml` (gh-deploy uses git internally), `release.yml` (CHANGELOG.md commit step).

---

## No Analog Found

Files with no close match in the codebase — use RESEARCH.md patterns instead:

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `LICENSE` | config | none | First license file; MIT boilerplate from RESEARCH.md |
| `CODE_OF_CONDUCT.md` | config | none | Contributor Covenant v2.1 — fetch from contributor-covenant.org/version/2/1/code_of_conduct/ |
| `SECURITY.md` | config | none | No vulnerability reporting process exists yet; prose authored from CONTEXT.md specifics |
| `docs/security.md` | doc | none | Content from CONTEXT.md specifics section (threat model, AES-256-GCM scope, header contract, key rotation) |
| `docs/deployment/auth-proxies.md` | doc | none | Config snippets for 5 proxy types must be researched/authored; not in codebase |
| `docs/deployment/polar-oauth-setup.md` | doc | none | Polar developer console walkthrough; no analog |
| `docs/reference/http-endpoints.md` | doc | none | Must be derived from actual route definitions in `internal/` |
| `docs/reference/db-schema.md` | doc | none | Must be derived from migration files in `internal/store/` |
| `.github/PULL_REQUEST_TEMPLATE.md` | config | none | Standard PR checklist; no analog |
| `.github/CODEOWNERS` | config | none | Minimal: `* @lmgarret` |
| `CHANGELOG.md` (initial stub) | config | none | Empty stub or header-only; git-cliff overwrites on first release |

---

## Metadata

**Analog search scope:** `.github/workflows/`, root config files (`Makefile`, `Dockerfile`, `docker-compose.yml`, `.golangci.yml`), `skill/polar-coach/`
**Files scanned:** 8 existing files read in full
**Pattern extraction date:** 2026-05-11
