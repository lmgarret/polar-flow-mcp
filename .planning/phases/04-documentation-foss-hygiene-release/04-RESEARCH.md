# Phase 4: Documentation, FOSS Hygiene, Release - Research

**Researched:** 2026-05-11
**Domain:** MkDocs Material, git-cliff, GitHub Actions (multi-arch Docker + Pages + Release), FOSS community files
**Confidence:** HIGH

---

## Summary

Phase 4 is a pure content and CI-configuration phase. No Go code changes are needed. The deliverables split into three independent workstreams: (1) an MkDocs Material documentation site authored in Markdown and deployed to GitHub Pages, (2) two new GitHub Actions workflows — `docs.yml` for Pages and `release.yml` for versioned multi-arch Docker images plus GitHub Releases generated with git-cliff, and (3) standard FOSS community files (LICENSE, README with badges, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, and `.github/` templates).

All technology choices are locked by CONTEXT.md. The research confirms the locked stack is correct and current. The biggest implementation risk is the git-cliff release body wiring: the recommended pattern uses `orhun/git-cliff-action@v4` with `steps.<id>.outputs.content` piped into `softprops/action-gh-release@v3` via an intermediate file or a `release_body` job output — NOT via `body_path: CHANGELOG.md` directly (CHANGELOG.md is the cumulative file; the release needs only the single-version section). The `--latest` flag on git-cliff generates only the most recent tag's content, which is what should go in the release body.

The docs workflow is simpler: `pip install mkdocs-material && mkdocs gh-deploy --force`. No Go toolchain needed. The FOSS file section is entirely prose authoring with no surprises, but three URLs must be hardcoded correctly: Contributor Covenant v2.1, Go Report Card for `github.com/lmgarret/polar-flow-mcp`, and GitHub Pages at `https://lmgarret.github.io/polar-flow-mcp`.

**Primary recommendation:** Mirror the existing `ci.yml` action version pins exactly. Write `release.yml` as a single job (no matrix needed) that runs test+lint, then builds multi-arch Docker, then generates changelog and creates the GitHub Release.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**GitHub Repository Identity (D-01)**
- GitHub org/repo: `lmgarret/polar-flow-mcp`
- GHCR image: `ghcr.io/lmgarret/polar-flow-mcp`
- GitHub Pages URL: `https://lmgarret.github.io/polar-flow-mcp`
- README badges use `lmgarret/polar-flow-mcp`
- Go Report Card: `https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp`
- CI workflows use `${{ github.repository }}` where possible; hardcoded references must use `lmgarret/polar-flow-mcp`

**Go module path (D-02)**
- `github.com/lm/polar-flow-mcp` in `go.mod` does NOT change — it is internal only.

**Release Docker Build (D-03)**
- Multi-arch: `linux/amd64` + `linux/arm64`
- Requires `docker/setup-qemu-action` and `docker/setup-buildx-action` steps before build

**Release Image Tags (D-04)**
- semver tag: `v1.0.0` → `ghcr.io/lmgarret/polar-flow-mcp:1.0.0`
- `latest` tag on every `v*` push
- Use `docker/metadata-action` with `type=semver,pattern={{version}}` and `type=raw,value=latest`

**Release Workflow Trigger (D-05)**
- `push: tags: ['v*']` — separate from `ci.yml`
- Should also run tests + lint before building

**Documentation Content (D-06)**
- Fully written prose — no skeleton stubs
- Diátaxis structure with complete pages for every auth proxy scenario

**No Versioned Docs (D-07)**
- Single `gh-pages` branch, no `mike`, no versioning — latest only

**Placeholder Images (D-08)**
- `![placeholder](images/<descriptive-name>.png)` at logical screenshot points
- No real screenshots required

**git-cliff CHANGELOG location (D-09)**
- `CHANGELOG.md` in repo root, committed to repository
- Generated/updated as part of release workflow before GitHub Release

**git-cliff commit groups (D-10)**
- `feat` → Features
- `fix` → Bug Fixes
- `docs` → Documentation
- `chore` → Maintenance
- Unreleased section auto-maintained
- `cliff.toml` in repo root with this grouping config

**Release body (D-11)**
- GitHub Release description populated from git-cliff output (same content as CHANGELOG.md entry for that version)

### Claude's Discretion

- Exact `cliff.toml` format and any additional commit types to include/exclude (test, refactor, ci, etc.)
- Exact wording of README badge order and quickstart text (must include 5-line docker-compose quickstart referencing the docs URL)
- `mkdocs.yml` configuration details (theme color, nav structure, plugins beyond `search`)
- Whether to include `CODEOWNERS` file
- Exact issue template fields beyond bug report / feature request labels

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DOC-01 | `docs/` tree follows Diátaxis structure: `index.md`, `getting-started.md`, `deployment/` subtree (docker-compose, auth-proxies, polar-oauth-setup), `usage.md`, `reference/` subtree (env vars, MCP tool schemas, HTTP endpoints, DB schema), `security.md`, `contributing.md` | MkDocs Material nav configuration; standard Diátaxis layout maps directly to mkdocs.yml `nav:` |
| DOC-02 | `docs/security.md` covers: threat model, encryption-at-rest scope, fail-closed rationale, header contract, key rotation plan | Pure prose — content from Phase 1 implementation knowledge |
| DOC-03 | `docs/deployment/auth-proxies.md` covers Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access with header contract config snippets | Pure prose — research shows config snippet structure for each proxy |
| DOC-04 | Placeholder images via `![placeholder](images/<descriptive-name>.png)` at logical screenshot points | No tooling needed — just Markdown image syntax in docs pages |
| DOC-05 | MkDocs Material site deploys to GitHub Pages via `workflows/docs.yml` on push to `main` | `mkdocs gh-deploy --force` in GitHub Actions with `permissions: contents: write`; Python setup required |
| FOSS-01 | `LICENSE` (MIT), `README.md` (badges + quickstart + docs link), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant v2.1), `SECURITY.md` | Standard FOSS files; badge URL formats verified |
| FOSS-02 | `.github/` directory contains `workflows/ci.yml` (existing), `workflows/release.yml`, `workflows/docs.yml`, `ISSUE_TEMPLATE/` (bug + feature), `PULL_REQUEST_TEMPLATE.md` | GitHub Actions patterns verified; issue template YAML format confirmed |
| FOSS-03 | Release workflow uses git-cliff to generate `CHANGELOG.md` on tag push; Docker image tagged with semver + `latest` | git-cliff-action v4 + softprops/action-gh-release v3; multi-arch build pattern confirmed |
</phase_requirements>

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Documentation site build | CI (GitHub Actions) | Local dev only | `mkdocs gh-deploy` runs in Actions; local `mkdocs serve` for authoring |
| Documentation hosting | CDN / Static (GitHub Pages) | — | Static HTML served from `gh-pages` branch |
| Docker image build + push | CI (GitHub Actions) | — | buildx + QEMU in Actions runner; no local build needed |
| GitHub Release creation | CI (GitHub Actions) | — | softprops/action-gh-release triggered by tag push |
| CHANGELOG generation | CI (GitHub Actions) | Local dev optional | git-cliff-action in release.yml; `cliff.toml` enables local use too |
| FOSS community files | Repo root / .github/ | — | Static files committed to repository, no runtime component |
| Badge data | External services | — | shields.io (CI, license, release), ghcr-badge.egpl.dev (image size), goreportcard.com |

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| mkdocs-material | 9.7.6 (latest, 2026-03-19) | Documentation site theme + build | Industry standard for Go/Python OSS docs; Diátaxis-friendly nav |
| git-cliff | 2.13.1 (latest, 2026-04-26) | CHANGELOG generation from conventional commits | Most widely adopted conventional-commits changelog generator |
| orhun/git-cliff-action | v4 | GitHub Actions step for git-cliff | Official action; exposes `outputs.content` for release body wiring |
| softprops/action-gh-release | v3 (requires Node 24) | Create GitHub Release with body | De-facto standard release action; supports `body_path` |
| docker/setup-qemu-action | v3 | Enable ARM64 emulation in CI runner | Required for multi-arch buildx |
| docker/setup-buildx-action | v3 | Enable Docker Buildx multi-platform | Required for `platforms: linux/amd64,linux/arm64` |

[VERIFIED: PyPI mkdocs-material, GitHub orhun/git-cliff releases, GitHub softprops/action-gh-release]

### Supporting (already pinned in existing ci.yml — reuse exactly)

| Library | Version | Purpose |
|---------|---------|---------|
| actions/checkout | v4 | Repo checkout (use `fetch-depth: 0` in release.yml for full history) |
| actions/setup-go | v5 | Go toolchain setup via `go-version-file: go.mod` |
| actions/setup-python | v5 | Python for MkDocs in docs.yml |
| actions/cache | v4 | pip cache in docs.yml |
| golangci/golangci-lint-action | v9 (version: v2.11) | Lint in release.yml test gate |
| docker/login-action | v3 | GHCR authentication |
| docker/metadata-action | v5 | Tag generation (semver + latest) |
| docker/build-push-action | v6 | Multi-arch build + push |

[VERIFIED: existing `.github/workflows/ci.yml`]

### Installation (docs workflow)

```bash
pip install mkdocs-material
```

No `requirements.txt` needed unless pinning — for a single-author project, unpinned is acceptable. The CI workflow uses the cache pattern to avoid repeated downloads.

---

## Architecture Patterns

### System Architecture Diagram

```
                    ┌─────────────────────────────────────────┐
                    │           git push (main branch)         │
                    └──────────────────┬──────────────────────┘
                                       │
                                       ▼
                          ┌────────────────────────┐
                          │   docs.yml workflow     │
                          │  (GitHub Actions)       │
                          │  pip install material   │
                          │  mkdocs gh-deploy       │
                          └────────────┬───────────┘
                                       │ deploys
                                       ▼
                          ┌────────────────────────┐
                          │  gh-pages branch        │
                          │  (GitHub Pages CDN)     │
                          │  lmgarret.github.io/    │
                          │  polar-flow-mcp         │
                          └────────────────────────┘

                    ┌─────────────────────────────────────────┐
                    │        git push tag v*                   │
                    └──────────────────┬──────────────────────┘
                                       │
                                       ▼
                          ┌────────────────────────────────────┐
                          │      release.yml workflow           │
                          │   1. test (CGO_ENABLED=0 go test)  │
                          │   2. lint (golangci-lint-action)   │
                          │   3. QEMU + Buildx setup           │
                          │   4. metadata-action → tags        │
                          │   5. build-push-action             │
                          │      linux/amd64 + linux/arm64     │
                          │   6. git-cliff-action --latest     │
                          │   7. commit CHANGELOG.md           │
                          │   8. action-gh-release             │
                          └────┬──────────────────────┬────────┘
                               │                      │
                               ▼                      ▼
                  ┌────────────────────┐  ┌────────────────────────┐
                  │  ghcr.io image:    │  │  GitHub Release:        │
                  │  :1.0.0 + :latest  │  │  tag v1.0.0             │
                  │  amd64 + arm64     │  │  body = cliff output    │
                  └────────────────────┘  └────────────────────────┘
```

### Recommended Project Structure (Phase 4 additions)

```
polar-flow-mcp/
├── cliff.toml               # git-cliff config (repo root)
├── mkdocs.yml               # MkDocs Material config (repo root)
├── CHANGELOG.md             # Generated and committed by release workflow
├── LICENSE                  # MIT
├── README.md                # Badges + quickstart + docs link
├── CONTRIBUTING.md          # Clone → make test → PR flow
├── CODE_OF_CONDUCT.md       # Contributor Covenant v2.1
├── SECURITY.md              # Vulnerability reporting
├── docs/
│   ├── index.md             # Project overview + key features
│   ├── getting-started.md   # Prerequisites → link Polar → first tool call
│   ├── usage.md             # All MCP tools + skill installation
│   ├── security.md          # Threat model, AES-256-GCM scope, header contract
│   ├── contributing.md      # Same content as CONTRIBUTING.md in prose form
│   ├── deployment/
│   │   ├── docker-compose.md      # Full docker-compose.yml walkthrough
│   │   ├── auth-proxies.md        # Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare
│   │   └── polar-oauth-setup.md   # Polar developer app registration
│   ├── reference/
│   │   ├── env-vars.md            # All env vars with defaults + examples
│   │   ├── mcp-tools.md           # Tool schemas (input/output)
│   │   ├── http-endpoints.md      # /healthz, /readyz, /oauth/login, /oauth/callback, /mcp
│   │   └── db-schema.md           # SQLite tables + column types
│   └── images/                    # Placeholder images (empty dir + .gitkeep)
└── .github/
    ├── workflows/
    │   ├── ci.yml           # Existing — do not modify
    │   ├── docs.yml         # NEW — GitHub Pages deploy
    │   └── release.yml      # NEW — tag-triggered release
    ├── ISSUE_TEMPLATE/
    │   ├── bug_report.yml   # NEW
    │   └── feature_request.yml  # NEW
    └── PULL_REQUEST_TEMPLATE.md  # NEW
```

### Pattern 1: GitHub Pages Workflow (docs.yml)

```yaml
# Source: https://squidfunk.github.io/mkdocs-material/publishing-your-site
name: Docs

on:
  push:
    branches: [main]

permissions:
  contents: write

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # needed for git-revision-date plugin if added later

      - name: Configure Git credentials
        run: |
          git config user.name github-actions[bot]
          git config user.email 41898282+github-actions[bot]@users.noreply.github.com

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

[CITED: squidfunk.github.io/mkdocs-material/publishing-your-site]

### Pattern 2: Release Workflow (release.yml)

Key structural decisions:
- Single job or two jobs (test-first job, then release job with `needs: test`). Single job is simpler for a small project; two jobs is cleaner.
- `fetch-depth: 0` is **required** for git-cliff — it needs full git history to compute the changelog.
- Use `orhun/git-cliff-action@v4` with `--latest` flag to generate only the current tag's section.
- Pass changelog content via job output (`outputs.content`) into `softprops/action-gh-release@v3` `body:` parameter (not `body_path:`).
- Commit CHANGELOG.md back to the repo **before** creating the release.

```yaml
# Source: Docker docs (multi-platform), orhun/git-cliff CD workflow, softprops/action-gh-release README
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: write
  packages: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # REQUIRED for git-cliff

      # --- Test gate (mirrors ci.yml) ---
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run tests
        run: CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11

      # --- Docker multi-arch build ---
      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=raw,value=latest

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}

      # --- CHANGELOG + GitHub Release ---
      - name: Generate changelog
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
          git diff --staged --quiet || git commit -m "chore(release): update CHANGELOG.md for ${{ github.ref_name }}"
          git push origin HEAD:main

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v3
        with:
          body: ${{ steps.git-cliff.outputs.content }}
          make_latest: true
```

[CITED: Docker docs multi-platform, git-cliff CD workflow, softprops README]

**Critical wiring note:** `steps.git-cliff.outputs.content` contains only the current release section (because of `--latest`). This is the correct input for the release body. `CHANGELOG.md` on disk at this point contains the cumulative full changelog (all versions), which is what gets committed to git.

### Pattern 3: mkdocs.yml Configuration

```yaml
# Source: squidfunk.github.io/mkdocs-material/getting-started
site_name: polar-flow-mcp
site_url: https://lmgarret.github.io/polar-flow-mcp
site_description: Multi-user MCP server wrapping the Polar AccessLink API
repo_url: https://github.com/lmgarret/polar-flow-mcp
repo_name: lmgarret/polar-flow-mcp

theme:
  name: material
  palette:
    primary: deep orange   # Polar brand color approximation
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

[CITED: squidfunk.github.io/mkdocs-material — nav + features]

**Note on social plugin:** The `social` plugin generates Open Graph cards but requires `cairosvg` and `pillow` as additional pip dependencies. Skip it for v1 — it adds installation complexity with minimal user benefit.

### Pattern 4: cliff.toml Configuration

```toml
# cliff.toml
# Source: git-cliff.org/docs/configuration

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
    { message = "^refactor", group = "Refactor", skip = false },
    { message = "^test", skip = true },
    { message = "^ci", skip = true },
    { message = "^style", skip = true },
]
tag_pattern = "v[0-9].*"
```

[CITED: git-cliff.org/docs/configuration, orhun/git-cliff GitHub]

**Tag strip prefix:** The template uses `trim_start_matches(pat="v")` to strip the `v` prefix from version display (D-10 decision). This is the standard approach in git-cliff templates.

### Pattern 5: Badge Formats for README.md

```markdown
[![CI](https://img.shields.io/github/actions/workflow/status/lmgarret/polar-flow-mcp/ci.yml?branch=main&label=CI)](https://github.com/lmgarret/polar-flow-mcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/lmgarret/polar-flow-mcp)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/lmgarret/polar-flow-mcp)](https://github.com/lmgarret/polar-flow-mcp/releases/latest)
[![Image Size](https://ghcr-badge.egpl.dev/lmgarret/polar-flow-mcp/size)](https://github.com/lmgarret/polar-flow-mcp/pkgs/container/polar-flow-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/lmgarret/polar-flow-mcp)](https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp)
```

[CITED: shields.io/badges/git-hub-actions-workflow-status, github.com/eggplants/ghcr-badge]

**Image size badge:** `ghcr-badge.egpl.dev` is the correct service for GHCR image size badges. shields.io does not natively support ghcr.io image size. The URL format is `https://ghcr-badge.egpl.dev/<owner>/<repo>/size`.

**CI badge:** The workflow filename (not the `name:` YAML field) goes in the URL — use `ci.yml`.

**Go Report Card:** Uses the ACTUAL repo path (`github.com/lmgarret/polar-flow-mcp`), not the Go module path. This is distinct from D-02.

### Pattern 6: GitHub Issue Templates (YAML format)

GitHub now supports YAML-based issue templates (`.github/ISSUE_TEMPLATE/*.yml`). These are preferred over legacy Markdown templates because they support structured fields.

```yaml
# .github/ISSUE_TEMPLATE/bug_report.yml
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

[CITED: docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests]

### Anti-Patterns to Avoid

- **`mike` for versioned docs:** D-07 locked this out — do not add `mike` or version switcher config.
- **Committing CHANGELOG.md before Docker build:** The CHANGELOG commit step must be AFTER the Docker push step. If git push fails, the release partially completes, but that is safer than blocking the Docker push.
- **Using `body_path: CHANGELOG.md` for the GitHub Release:** CHANGELOG.md is the cumulative file (all versions). The release body should contain only the current version section, which is `steps.git-cliff.outputs.content` from `--latest`.
- **Pinning `pip install mkdocs-material==9.7.6`:** Use unpinned `pip install mkdocs-material` with the Actions cache. Pinning in a YAML step string is fragile; use a `requirements.txt` if pinning is needed.
- **`docker/setup-qemu-action@v3` missing from release.yml:** Without QEMU setup, the buildx multi-arch build will fail silently for arm64 targets. It must precede `setup-buildx-action`.
- **Social plugin without dependencies:** The `social` plugin requires `cairosvg` and `pillow`. Adding `- social` to plugins without `pip install "mkdocs-material[imaging]"` causes the build to fail.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Multi-arch Docker manifest | Shell scripts with `docker manifest create` | `docker/build-push-action@v6` with `platforms:` | buildx handles manifest list automatically; manual manifest creation breaks layer caching |
| CHANGELOG generation | Custom git log parsing | `orhun/git-cliff-action@v4` | Handles merge commits, squash commits, first-time setup, Tera templating |
| GitHub Release creation | `gh release create` shell step | `softprops/action-gh-release@v3` | Handles asset upload, pre-release detection, `make_latest` flag; more reliable than gh CLI in Actions |
| Documentation nav from filesystem | Auto-detected nav | Explicit `nav:` in mkdocs.yml | Auto-detection ordering is alphabetical and does not match Diátaxis intent |
| Image size badge | GitHub API + custom endpoint | `ghcr-badge.egpl.dev` | Maintained external service with correct GHCR authentication |

---

## Common Pitfalls

### Pitfall 1: git-cliff missing full history

**What goes wrong:** `git-cliff` outputs an empty CHANGELOG or only the current commit.
**Why it happens:** `actions/checkout@v4` defaults to `fetch-depth: 1` (shallow clone). git-cliff needs the full tag history.
**How to avoid:** Always set `fetch-depth: 0` in the `actions/checkout` step of `release.yml`.
**Warning signs:** CHANGELOG.md contains only one commit entry; `git log --oneline` in the runner shows only the tagged commit.

### Pitfall 2: CHANGELOG.md commit triggers docs.yml

**What goes wrong:** The `chore(release): update CHANGELOG.md` commit pushed by release.yml triggers `docs.yml`, which re-deploys docs unnecessarily.
**Why it happens:** `docs.yml` triggers on all pushes to `main`.
**How to avoid:** Add `[skip ci]` to the CHANGELOG commit message, or add a `paths-ignore: ['CHANGELOG.md']` filter to `docs.yml`. The skip-ci approach is simpler.

### Pitfall 3: metadata-action generates wrong semver tag format

**What goes wrong:** Image pushed as `v1.0.0` instead of `1.0.0`.
**Why it happens:** `type=semver,pattern={{version}}` strips the `v` prefix correctly. Using `type=ref,event=tag` does NOT strip the prefix.
**How to avoid:** Use `type=semver,pattern={{version}}` exactly as shown in D-04.
**Warning signs:** Image tag in GHCR shows `v1.0.0` not `1.0.0`.

### Pitfall 4: MkDocs gh-deploy fails — no write permission

**What goes wrong:** `mkdocs gh-deploy --force` fails with a permission error.
**Why it happens:** The workflow lacks `permissions: contents: write`.
**How to avoid:** Add `permissions: contents: write` at job or workflow level in `docs.yml`.
**Warning signs:** GitHub Actions log shows "403 Permission denied" on git push to gh-pages.

### Pitfall 5: Go Report Card shows wrong module

**What goes wrong:** Go Report Card badge links to `github.com/lm/polar-flow-mcp` which is the Go module path but not the GitHub repo path.
**Why it happens:** Confusion between D-01 (GitHub identity) and D-02 (Go module path).
**How to avoid:** Go Report Card URL uses the GitHub repo path: `https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp`. The Go module path is irrelevant to the badge.

### Pitfall 6: Contributor Covenant version confusion

**What goes wrong:** CODE_OF_CONDUCT.md uses v2.0 text but links to v2.1 URL.
**Why it happens:** v2.1 was released after many templates were written; old boilerplate is v2.0.
**How to avoid:** Fetch the v2.1 text directly from `https://www.contributor-covenant.org/version/2/1/code_of_conduct/` — do not copy from other repos.

### Pitfall 7: release.yml fails on first tag — no previous CHANGELOG

**What goes wrong:** git-cliff `--latest` generates an empty output when there is no previous tag to diff against.
**Why it happens:** `--latest` means "from previous tag to current tag". On the very first `v1.0.0` tag with no previous tag, git-cliff uses `--unreleased` behavior automatically.
**How to avoid:** No action needed — git-cliff handles the first-tag case correctly. Do verify in testing with a test tag before the real `v1.0.0` push.

---

## Code Examples

### Complete release.yml skeleton (verified pattern)

```yaml
# Source: merged from Docker docs, git-cliff CD workflow, softprops README
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: write
  packages: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run tests
        run: CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...

      - name: Lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=raw,value=latest

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}

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

### MIT LICENSE boilerplate

```text
MIT License

Copyright (c) 2026 LM. Garret

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `actions/create-release` (deprecated) | `softprops/action-gh-release@v3` | 2022 | `actions/create-release` is unmaintained; v3 requires Node 24 |
| `docker manifest create` shell scripts | `docker/build-push-action` with `platforms:` | ~2021 | buildx is now the standard; manual manifests are error-prone |
| `mike` for versioned MkDocs | Single branch gh-deploy for v1 | N/A | D-07 decision; mike is appropriate at v2+ when breaking changes exist |
| Markdown issue templates | YAML issue templates | 2021 (GitHub) | YAML templates support required fields and structured input |
| `git log --pretty` manual scripts | `git-cliff` with `cliff.toml` | ~2022 | git-cliff handles edge cases (merge commits, first release, unreleased) |

**Deprecated/outdated:**
- `actions/create-release`: Unmaintained since 2021. Do not use.
- Legacy `.github/ISSUE_TEMPLATE/bug_report.md` (Markdown format): Still works but YAML format provides validation and required-field enforcement.
- `type=ref,event=tag` in metadata-action for semver: Does not strip `v` prefix. Use `type=semver,pattern={{version}}`.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `softprops/action-gh-release@v3` is the correct current version | Standard Stack | Could be @v2 if v3's Node 24 requirement causes runner issues — v2 is the safe fallback |
| A2 | `ghcr-badge.egpl.dev` is a maintained, stable service | Code Examples (badges) | Service may go offline; image size badge would break (cosmetic only, not blocking) |
| A3 | `orhun/git-cliff-action@v4` works correctly on first-tag releases | Common Pitfalls | If git-cliff v4 changed `--latest` behavior, the release body could be empty |
| A4 | GitHub Pages is already enabled on the `lmgarret/polar-flow-mcp` repository with source set to `gh-pages` branch | Entire DOC-05 | Must be manually enabled in repo Settings → Pages before the workflow can deploy |

---

## Open Questions

1. **GitHub Pages repository setting**
   - What we know: `docs.yml` pushes to `gh-pages` branch via `mkdocs gh-deploy --force`
   - What's unclear: Whether GitHub Pages is configured in the repo settings (source branch = gh-pages). This is a one-time manual click in Settings → Pages.
   - Recommendation: Plan should include a verification step confirming Pages is enabled. If not enabled, the workflow will push to the branch but the site will not be served.

2. **CHANGELOG.md initial state**
   - What we know: git-cliff generates CHANGELOG.md from scratch on first run.
   - What's unclear: Whether a `CHANGELOG.md` stub should exist in the repo before the first release tag, or if git-cliff creates it.
   - Recommendation: Include an empty `CHANGELOG.md` (or with just the header) committed to main before tagging v1.0.0. git-cliff will overwrite it. This avoids a "file not found" edge case.

3. **CODEOWNERS file**
   - What we know: CONTEXT.md marks this as Claude's Discretion.
   - Recommendation: Include a minimal `CODEOWNERS` with `* @lmgarret`. It costs nothing and enables auto-assignment of reviews.

---

## Environment Availability

> Step 2.6: No external runtime dependencies introduced by Phase 4 code. The workflows run entirely in GitHub Actions runners. The only local tooling needed is `mkdocs serve` for doc authoring, which is optional.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| GitHub Actions runners | All workflows | ✓ | ubuntu-latest | — |
| Python 3.x | docs.yml (mkdocs) | ✓ (Actions runner) | 3.x via setup-python | — |
| Docker buildx + QEMU | release.yml | ✓ (setup-qemu-action + setup-buildx-action) | latest via actions | — |
| Go toolchain | release.yml (test+lint) | ✓ (setup-go@v5 + go.mod) | 1.26.2 | — |

**Local authoring (optional):**
```bash
pip install mkdocs-material
mkdocs serve   # live preview at http://127.0.0.1:8000
```

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package + `go test` |
| Config file | None — `Makefile` targets |
| Quick run command | `make test` (`CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...`) |
| Full suite command | `make test && make lint` |

**Note:** Phase 4 introduces no new Go code. All existing tests continue to apply. The validation focus is on workflow correctness (dry-run via `act` or first push to a test branch) and prose completeness.

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DOC-01 | `docs/` tree exists with all required pages | Manual / link-check | `mkdocs serve` + `mkdocs build --strict` | ❌ Wave 0 (create docs/) |
| DOC-02 | security.md covers all required topics | Manual review | N/A — prose review | ❌ Wave 0 |
| DOC-03 | auth-proxies.md has real config snippets for all 5 proxies | Manual review | N/A — prose review | ❌ Wave 0 |
| DOC-04 | Placeholder images use correct syntax at logical points | Manual review | `mkdocs build --strict` (broken image refs fail) | ❌ Wave 0 |
| DOC-05 | docs.yml deploys to GitHub Pages on push to main | Integration / CI | Push to main and verify Pages URL serves | ❌ Wave 0 (create docs.yml) |
| FOSS-01 | LICENSE, README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY all exist | Manual / file check | `ls LICENSE README.md CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md` | ❌ Wave 0 |
| FOSS-02 | ISSUE_TEMPLATE + PR_TEMPLATE + release.yml exist in .github/ | Manual / file check | `ls .github/ISSUE_TEMPLATE/ .github/PULL_REQUEST_TEMPLATE.md .github/workflows/release.yml` | ❌ Wave 0 |
| FOSS-03 | Tag push triggers release.yml; multi-arch image + GitHub Release created | Integration / CI | Push `v0.0.1-test` tag and inspect GHCR + Releases | ❌ Wave 0 (create release.yml + cliff.toml) |

### Sampling Rate

- **Per task commit:** `make test` (existing tests must continue to pass — no Go changes expected)
- **Per wave merge:** `make test && make lint && mkdocs build --strict`
- **Phase gate:** All files exist, `mkdocs build --strict` passes, README badges resolve, `docs.yml` and `release.yml` pass YAML lint

### Wave 0 Gaps

- [ ] `docs/` directory with all Diátaxis pages — covers DOC-01..04
- [ ] `mkdocs.yml` — covers DOC-01, DOC-05
- [ ] `.github/workflows/docs.yml` — covers DOC-05
- [ ] `.github/workflows/release.yml` — covers FOSS-02, FOSS-03
- [ ] `cliff.toml` — covers FOSS-03
- [ ] `LICENSE`, `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md` — covers FOSS-01
- [ ] `.github/ISSUE_TEMPLATE/bug_report.yml` + `feature_request.yml` — covers FOSS-02
- [ ] `.github/PULL_REQUEST_TEMPLATE.md` — covers FOSS-02

*(No test infrastructure gaps — existing Go tests are unaffected by Phase 4)*

---

## Security Domain

> Phase 4 introduces no server code. Security considerations are limited to CI/CD supply chain hygiene.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | no | — |
| V5 Input Validation | no | — |
| V6 Cryptography | no | — |
| Supply chain (CI) | yes | Pin action versions; use `GITHUB_TOKEN` not PAT |

### CI Supply Chain Notes

- All GitHub Actions use `GITHUB_TOKEN` (scoped to the repo, ephemeral). No PATs needed.
- `permissions: contents: write, packages: write` is the minimum required scope for the release workflow — do not use `permissions: write-all`.
- Action versions: reuse the same pinned versions from `ci.yml` for all shared actions. The `orhun/git-cliff-action@v4` and `softprops/action-gh-release@v3` are pinned by major version tag — acceptable for a personal project; a high-security environment would pin to SHA.

---

## Sources

### Primary (HIGH confidence)
- `/websites/squidfunk_github_io_mkdocs-material` (Context7) — nav configuration, GitHub Pages workflow, plugin setup
- `orhun/git-cliff` (Context7 + GitHub raw CD workflow) — cliff.toml structure, Actions integration, `--latest` flag, `outputs.content`
- `github.com/softprops/action-gh-release` (WebFetch) — v3 current version, `body:` parameter, `make_latest` input
- `docs.docker.com/build/ci/github-actions/multi-platform/` (WebFetch) — QEMU + buildx + build-push-action workflow pattern
- Existing `.github/workflows/ci.yml` (Read tool) — action versions to mirror exactly

### Secondary (MEDIUM confidence)
- shields.io badge URL format for GitHub Actions workflow status (WebSearch verified with shields.io docs)
- `github.com/eggplants/ghcr-badge` (WebFetch) — ghcr-badge service URL format for image size badge
- shields.io GitHub Release badge and Go Report Card badge URL formats (WebSearch, cross-referenced with shields.io)

### Tertiary (LOW confidence)
- Contributor Covenant v2.1 URL (`contributor-covenant.org/version/2/1/code_of_conduct/`) — confirmed via WebSearch as the authoritative source; content not fetched in this session

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — versions verified against PyPI (mkdocs-material 9.7.6), GitHub releases (git-cliff 2.13.1), softprops README (v3)
- GitHub Actions workflows: HIGH — patterns verified from official Docker docs and git-cliff's own CD workflow
- FOSS file content: HIGH for structure, MEDIUM for prose (content is authored, not researched)
- Badge URLs: HIGH for CI + license + release (shields.io documented), MEDIUM for image size (third-party ghcr-badge service)

**Research date:** 2026-05-11
**Valid until:** 2026-08-11 (stable ecosystem — action major versions rarely break in <90 days)
