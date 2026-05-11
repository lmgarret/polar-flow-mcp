# Phase 4: Documentation, FOSS Hygiene, Release - Context

**Gathered:** 2026-05-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Make the project publicly releasable: MkDocs Material documentation site (Diátaxis structure, fully written prose), GitHub Pages deploy workflow, release automation with multi-arch Docker + git-cliff CHANGELOG generation, and complete FOSS community infrastructure (LICENSE, README with badges, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, .github/ templates).

Delivers: DOC-01, DOC-02, DOC-03, DOC-04, DOC-05, FOSS-01, FOSS-02, FOSS-03.

</domain>

<decisions>
## Implementation Decisions

### GitHub Repository Identity

- **D-01:** The actual GitHub path is `lmgarret/polar-flow-mcp` (NOT `lm/polar-flow-mcp` which is only the Go module path in go.mod). All GitHub-facing output must use `lmgarret/polar-flow-mcp`:
  - GHCR image: `ghcr.io/lmgarret/polar-flow-mcp`
  - GitHub Pages URL: `https://lmgarret.github.io/polar-flow-mcp`
  - README badges (CI, license, release, image size, Go report card) use `lmgarret/polar-flow-mcp`
  - Go report card badge: `https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp`
  - CI workflows use `${{ github.repository }}` which resolves correctly at runtime; hardcoded references must use `lmgarret/polar-flow-mcp`
- **D-02:** The Go module path in `go.mod` (`github.com/lm/polar-flow-mcp`) does NOT need to change — it is internal only and not visible to end users.

### Release Docker Build

- **D-03:** Release workflow (`workflows/release.yml`) builds a **multi-arch Docker image: linux/amd64 + linux/arm64**. Requires `docker/setup-qemu-action` and `docker/setup-buildx-action` steps before the build step.
- **D-04:** Release image is tagged with both **semver tag** (`v1.0.0` → `ghcr.io/lmgarret/polar-flow-mcp:1.0.0`) and **`latest`**. Use `docker/metadata-action` with `type=semver,pattern={{version}}` and `type=raw,value=latest`. Both tags pushed on every `v*` tag push.
- **D-05:** Release workflow triggers on `push: tags: ['v*']`. It is separate from `ci.yml` (which triggers on branch push/PR). The release workflow should also run tests + lint before building the image (or at minimum depend on a passing CI run — `needs: []` is fine since tag pushes on main imply CI already passed).

### Documentation Content

- **D-06:** Documentation content must be **fully written prose** — each Diátaxis page is complete enough that a self-hoster can deploy from the docs alone. No skeleton stubs. Auth proxy section covers Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access with real config snippets. Reference section covers all env vars, MCP tool schemas, HTTP endpoints, DB schema.
- **D-07:** **No versioning (no mike)** — docs.yml pushes to `gh-pages` branch on every push to `main`, serving a single latest version. Versioned docs can be added when v2 ships.
- **D-08:** Placeholder images per DOC-04: use `![placeholder](images/<descriptive-name>.png)` at logical screenshot points (Polar app registration, auth proxy login flow, Claude using tools). No real screenshots required in Phase 4.

### git-cliff Changelog

- **D-09:** `CHANGELOG.md` lives in **repo root** and is committed to the repository. git-cliff generates/updates it as part of the release workflow before creating the GitHub Release.
- **D-10:** git-cliff uses **standard conventional commit groups**:
  - `feat` → Features
  - `fix` → Bug Fixes
  - `docs` → Documentation
  - `chore` → Maintenance
  - An Unreleased section is auto-maintained. A `cliff.toml` (or `.cliff.toml`) must be added to the repo root with this grouping config.
- **D-11:** The GitHub Release description body is populated from git-cliff output (same content as CHANGELOG.md entry for that version).

### Claude's Discretion

- Exact `cliff.toml` format and any additional commit types to include/exclude (test, refactor, ci, etc.)
- Exact wording of README badges order and quickstart text (must include 5-line docker-compose quickstart referencing the docs URL)
- `mkdocs.yml` configuration details (theme color, nav structure, plugins beyond `search`)
- Whether to include `CODEOWNERS` file
- Exact issue template fields beyond bug report / feature request labels

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project constraints and requirements
- `.planning/PROJECT.md` — tech stack, key decisions, CI pattern to mirror, MkDocs Material decision
- `.planning/REQUIREMENTS.md` — DOC-01..05, FOSS-01..03 (full requirement text with exact page names and content spec)
- `CLAUDE.md` — critical decisions table; CI structure to mirror exactly

### Existing CI to mirror
- `.github/workflows/ci.yml` — three-job pattern (test + lint → docker); golangci-lint-action@v9 v2.11; ghcr.io push with `${{ github.repository }}`; **release.yml must follow the same action versions and structure**

### FOSS file references
- Contributor Covenant (CODE_OF_CONDUCT.md): https://www.contributor-covenant.org/version/2/1/code_of_conduct/ — use v2.1
- git-cliff: https://git-cliff.org/docs/configuration — for cliff.toml structure

### GitHub identity (hardcode these values in all files)
- GitHub org/repo: `lmgarret/polar-flow-mcp`
- GHCR image: `ghcr.io/lmgarret/polar-flow-mcp`
- GitHub Pages: `https://lmgarret.github.io/polar-flow-mcp`
- Go module path (internal only, do not change): `github.com/lm/polar-flow-mcp`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `.github/workflows/ci.yml` — copy action versions (`actions/checkout@v4`, `actions/setup-go@v5`, `golangci-lint-action@v9`, `docker/login-action@v3`, `docker/metadata-action@v5`, `docker/build-push-action@v6`) into `release.yml` and `docs.yml`
- `Makefile` — `make test` and `make lint` targets; `CONTRIBUTING.md` can reference these directly
- `docker-compose.yml` — reference in README quickstart (but note it ships with `AUTH_PROXY=unconfigured` intentionally)
- `skill/polar-coach/SKILL.md` — reference the two installation paths in docs (`usage.md` or a dedicated skill section)

### Established Patterns
- CI uses `ghcr.io/${{ github.repository }}` — release.yml should follow the same; `${{ github.repository }}` resolves to `lmgarret/polar-flow-mcp` at runtime
- Go 1.26 from `go.mod` (go-version-file: go.mod) — docs.yml Go setup step should follow same pattern if it needs Go
- Three-job structure in ci.yml — release.yml can simplify (no separate lint job needed if tag pushes imply CI passed)

### Integration Points
- `docs/` — new top-level directory; `mkdocs.yml` at repo root
- `.github/workflows/docs.yml` — new workflow alongside `ci.yml`
- `.github/workflows/release.yml` — new workflow alongside `ci.yml`
- `.github/ISSUE_TEMPLATE/` — new directory
- `CHANGELOG.md`, `LICENSE`, `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md` — all new repo-root files

</code_context>

<specifics>
## Specific Ideas

- README quickstart must be exactly 5 lines covering the happy path: clone → copy env config → set encryption key → docker compose up → verify with Claude
- `docs/deployment/auth-proxies.md` must include real config snippets for Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access (not just a list of names)
- `docs/security.md` covers: threat model, what AES-256-GCM at-rest protects (key compromise ≠ DB compromise), why server refuses to start by default, header contract (identity header + shared secret), key rotation plan (v1: no rotation; v2: key_version column ready)
- git-cliff tag strip prefix: `v` (so `v1.0.0` → `1.0.0` in CHANGELOG)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 4-Documentation, FOSS Hygiene, Release*
*Context gathered: 2026-05-11*
