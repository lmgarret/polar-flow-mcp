# Roadmap: polar-flow-mcp

**4 phases** | **40 v1 requirements** | Generated: 2026-05-03

## Phase Overview

| # | Phase | Goal | Requirements | Plans |
|---|-------|------|--------------|-------|
| 1 | Project Foundation and Security Skeleton | 5/5 | Complete    | 2026-05-05 |
| 2 | OAuth Link Flow + UserInfo | Users can link their Polar account and verify the link from Claude | OAUTH-01..05, MCP-01 | 3 |
| 3 | Core MCP Tools + Bundled Skill | 3/3 | Complete   | 2026-05-11 |
| 4 | Documentation, FOSS Hygiene, Release | Project is publicly releasable with docs, changelog, and FOSS community infrastructure | DOC-01..05, FOSS-01..03 | 3 |

---

## Phases

- [x] **Phase 1: Project Foundation and Security Skeleton** — Go scaffold, fail-closed auth, SQLite + crypto, Docker image, CI pipeline
- [x] **Phase 2: OAuth Link Flow + UserInfo** — Polar OAuth2 login/callback, encrypted token storage, get_user_info tool
- [x] **Phase 3: Core MCP Tools + Bundled Skill** — create/list/delete training targets, coaching-language mapping, polar-coach skill (completed 2026-05-11)
- [ ] **Phase 4: Documentation, FOSS Hygiene, Release** — MkDocs site, GitHub Pages, release workflow, FOSS files

---

## Phase Details

### Phase 1: Project Foundation and Security Skeleton

**Goal:** A compilable, containerizable server exists with all irreversible security and infrastructure decisions locked in — middleware ordering, crypto schema, SQLite concurrency model, and CI pipeline — so every subsequent phase builds on a stable, auditable base.

**Depends on:** Nothing (first phase)

**Requirements:** FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, FOUND-06, FOUND-07, FOUND-08, FOUND-09, FOUND-10, SERV-01, SERV-02, SERV-03, SERV-04, SERV-05, SERV-06, SERV-07, SERV-08

**Plans:** 3 plans
5/5 plans complete
2. [DONE 2026-05-04] Config validation and startup security — `config` package, `AUTH_PROXY`/`PROXY_SHARED_SECRET`/`KEY_PROVIDER`/`ENCRYPTION_KEY` fail-closed checks, startup security banner (FOUND-02..04, FOUND-10, SERV-05)
3. [DONE 2026-05-04] SQLite + crypto foundation — WAL dual-pool setup, `golang-migrate` embedded SQL migrations (3-table schema), AES-256-GCM `KeyProvider` interface with `env` and `file` implementations (FOUND-05..09)
4. [DONE 2026-05-04] HTTP server and middleware — stdlib `ServeMux`, `subtle.ConstantTimeCompare` secret middleware, `/healthz`, `/readyz`, `StreamableHTTP` MCP mount at `/mcp`, identity header injection via `WithHTTPContextFunc` (SERV-01..05)
5. [DONE 2026-05-04] Docker image and CI pipeline — multi-stage `Dockerfile` (`golang:1.26-alpine` → `scratch`), `docker-compose.yml` with fail-loud defaults, CI workflow (`test` + `lint` + `docker` jobs) mirroring karaclean, `ghcr.io` push with `latest` + SHA tags (SERV-06..08)

**Success Criteria:**
1. `CGO_ENABLED=0 go build ./...` produces a single static binary with no errors and `go test -race -count=1 ./...` passes
2. `docker build .` produces an image under 25 MB; `docker run` with `AUTH_PROXY=unconfigured` exits non-zero and prints a message containing "AUTH_PROXY" and a docs pointer
3. `docker compose up` with the shipped `docker-compose.yml` (which has `AUTH_PROXY=unconfigured`) fails loudly with the startup error — not silently
4. Running the server with valid config emits a startup log line naming the trusted identity header, the required secret header, and the declared auth proxy
5. `GET /readyz` returns 503 with named failing checks when any of `AUTH_PROXY`, `PROXY_SHARED_SECRET`, DB connection, or encryption key is absent; returns 200 when all are satisfied

**UI hint:** no

---

### Phase 2: OAuth Link Flow + UserInfo

**Goal:** A user can navigate to `/oauth/login` from behind their reverse proxy, authorize with Polar, and have their encrypted token stored — then verify the link by calling `get_user_info` from Claude.

**Depends on:** Phase 1

**Requirements:** OAUTH-01, OAUTH-02, OAUTH-03, OAUTH-04, OAUTH-05, MCP-01

**Plans:** 4 plans
- [x] 02-01-PLAN.md — Config Polar fields + store CRUD (CreateOAuthState/ConsumeOAuthState/UpsertUser/GetPolarUserID/UpsertToken) + tests (OAUTH-01, OAUTH-02, OAUTH-05 partial) [DONE 2026-05-10]
- [x] 02-02-PLAN.md — Polar client (ExchangeCode/RegisterUser) + OAuth Login/Callback handlers + main.go wiring + tests (OAUTH-01..05) [DONE 2026-05-10]
- [x] 02-03-PLAN.md — get_user_info MCP tool + RegisterTools signature update + phase-gate verification (MCP-01) [DONE 2026-05-10]
- [x] 02-04-PLAN.md — Gap closure: CR-01 expired-state delete-before-check, CR-02 RegisterUser member-id, CR-03 testexports.go production leak (OAUTH-02, OAUTH-04, OAUTH-05) [DONE 2026-05-10]

**Success Criteria:**
1. Visiting `/oauth/login` behind a correctly configured proxy redirects to `flow.polar.com` with a `state` parameter; a second visit generates a different state value
2. Completing the Polar authorization flow results in `GET /readyz`-equivalent DB reachability and a row in `polar_tokens`; the stored token BLOB has a 12-byte nonce prepended (verifiable by column length)
3. Calling `get_user_info` in Claude for a linked user returns the Polar user ID; calling it for an unlinked user returns a human-readable message (not a 500 or empty response)
4. Replaying the OAuth callback with the same state token returns an error — state is consumed on first use

**UI hint:** no

---

### Phase 3: Core MCP Tools + Bundled Skill

**Goal:** A user can dictate a training session in plain language to Claude ("5×1km threshold with 2min recovery for Thursday"), have it created in Polar Flow, view upcoming targets, and delete incorrect ones — with a bundled skill guiding Claude's behavior without any user configuration.

**Depends on:** Phase 2

**Requirements:** MCP-02, MCP-03, MCP-04, MCP-05, MCP-06, MCP-07, SKILL-01, SKILL-02, SKILL-03, SKILL-04

**Plans:** 3/3 plans complete
- [x] 03-01-PLAN.md — `create_training_target` tool: store.GetEncryptedToken, RegisterTools(cipher), polar.Client.CreateTrainingTarget with struct types, handler with flat-to-tree phase transform + label→zone mapping (MCP-02, MCP-03, MCP-06, MCP-07) [DONE 2026-05-11]
- [x] 03-02-PLAN.md — `list_training_targets` and `delete_training_target` tools: polar.Client.ListTrainingTargets + DeleteTrainingTarget (with ErrTargetNotFound sentinel), both handlers with identity/decrypt/unlinked-error pattern (MCP-04, MCP-05, MCP-06, MCP-07) [DONE 2026-05-11]
- [x] 03-03-PLAN.md — Bundled `skill/polar-coach/SKILL.md`: trigger, 4 tool names, HR zone table, worked examples (5×1km threshold + iterative marathon plan), when-NOT-to-call, safe degradation, both installation paths (SKILL-01..04)

**Success Criteria:**
1. Saying "create a 5×1km threshold session for next Thursday at 18:00" in Claude produces a Polar training target visible in the Polar Flow app with 5 repeat phases at Z4 intensity
2. `list_training_targets` returns a readable list of upcoming sessions; calling it with no linked account returns a clear error message, not a panic or 500
3. `delete_training_target` with a valid target ID removes it from Polar Flow and returns a confirmation; an invalid ID returns a clear "not found" message
4. `go test -race -count=1 ./...` passes with concurrent simulated user requests — no data races on handler state

**UI hint:** no

---

### Phase 4: Documentation, FOSS Hygiene, Release

**Goal:** The project is publicly releasable: documentation covers every deployment scenario a self-hoster will encounter, the release workflow produces versioned multi-arch Docker images automatically, and FOSS community infrastructure is in place for contributors.

**Depends on:** Phase 3

**Requirements:** DOC-01, DOC-02, DOC-03, DOC-04, DOC-05, FOSS-01, FOSS-02, FOSS-03

**Plans:** 3 plans
1. MkDocs Material documentation site — Diátaxis structure (`index.md`, `getting-started.md`, `deployment/` subtree, `usage.md`, `reference/` subtree, `security.md`, `contributing.md`); `security.md` covers threat model, encryption-at-rest scope, fail-closed rationale, header contract, key rotation plan; `deployment/auth-proxies.md` covers Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access; placeholder images at logical screenshot points (DOC-01..04)
2. GitHub Pages deploy workflow and release automation — `workflows/docs.yml` (pushes to `main` deploy to Pages), `workflows/release.yml` (triggered on `v*` tags: multi-arch Docker build, GitHub Release creation, `git-cliff` CHANGELOG generation) (DOC-05, FOSS-02, FOSS-03)
3. FOSS hygiene files — `LICENSE` (MIT), `README.md` (CI/license/release/image-size/Go-report-card badges, screenshot placeholder, 5-line quickstart, docs link), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant), `SECURITY.md`, `.github/ISSUE_TEMPLATE/` (bug + feature), `.github/PULL_REQUEST_TEMPLATE.md` (FOSS-01, FOSS-02)

**Success Criteria:**
1. `mkdocs serve` renders the full site locally with no broken links; the `docs.yml` workflow deploys it to GitHub Pages on a push to `main`
2. Pushing a `v1.0.0` tag triggers the release workflow: a multi-arch Docker image appears at `ghcr.io/<repo>:1.0.0` and `ghcr.io/<repo>:latest`, and a GitHub Release is created with a `git-cliff`-generated changelog
3. A new contributor can follow `CONTRIBUTING.md` from clone to passing `make test` and `make lint` without additional instructions
4. `README.md` badges all resolve (CI status, license, latest release, image size, Go report card) and the 5-line quickstart references the correct docs URL

**UI hint:** no

---

## Progress Table

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Project Foundation and Security Skeleton | 5/5 | Complete | 2026-05-04 |
| 2. OAuth Link Flow + UserInfo | 4/4 | Complete | 2026-05-10 |
| 3. Core MCP Tools + Bundled Skill | 2/3 | In progress | - |
| 4. Documentation, FOSS Hygiene, Release | 0/3 | Not started | - |
