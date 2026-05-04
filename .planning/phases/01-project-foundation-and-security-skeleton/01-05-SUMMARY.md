---
phase: 01-project-foundation-and-security-skeleton
plan: 05
subsystem: infra
tags: [docker, dockerfile, github-actions, ci, ghcr, golangci-lint, scratch-image]

# Dependency graph
requires:
  - phase: 01-project-foundation-and-security-skeleton
    provides: compilable Go binary (cmd/polar-flow-mcp/main.go) built in prior plans

provides:
  - Multi-stage Dockerfile (golang:1.26-alpine → FROM scratch), 11.46 MB final image
  - docker-compose.yml with AUTH_PROXY=unconfigured fail-closed sentinel
  - GitHub Actions CI: test + lint → docker jobs pushing to ghcr.io

affects:
  - Phase 2 deployment: Docker image available at ghcr.io/lm/polar-flow-mcp
  - All future phases: CI pipeline validates every commit

# Tech tracking
tech-stack:
  added:
    - Docker multi-stage build (golang:1.26-alpine builder, FROM scratch final)
    - GitHub Actions (actions/checkout@v4, actions/setup-go@v6, golangci/golangci-lint-action@v9)
    - docker/login-action@v3, docker/metadata-action@v5, docker/build-push-action@v6
  patterns:
    - FROM scratch final image: binary-only container with zero OS attack surface
    - ca-certificates.crt copied from builder stage to enable outbound HTTPS
    - docker-compose.yml ships deliberately broken to force operator configuration
    - CI test/lint gate before docker: fail fast, never push broken images

key-files:
  created:
    - Dockerfile
    - docker-compose.yml
    - .github/workflows/ci.yml
  modified: []

key-decisions:
  - "FROM scratch final stage (not alpine/distroless): zero OS tools, binary-only attack surface per T-05-01"
  - "ca-certificates.crt copied from builder: required for outbound HTTPS to Polar API per Pitfall 7"
  - "docker-compose.yml ships AUTH_PROXY=unconfigured: fail-closed UX contract for operators per T-05-03"
  - "CI docker job push: github.event_name != pull_request — PRs build but don't push to registry"
  - "go-version-file: go.mod in test and lint jobs: auto-follows toolchain bumps without CI changes"

patterns-established:
  - "Scratch image pattern: copy only ca-certificates.crt from builder; no shell, no package manager"
  - "CI gate pattern: docker job needs [test, lint]; broken code never reaches registry"

requirements-completed: [SERV-06, SERV-07, SERV-08]

# Metrics
duration: 5min
completed: 2026-05-04
---

# Phase 1 Plan 05: Docker + CI Summary

**Multi-stage Dockerfile (golang:1.26-alpine → FROM scratch, 11.46 MB) with AUTH_PROXY=unconfigured fail-closed docker-compose.yml and three-job GitHub Actions CI (test + lint → docker push to ghcr.io with latest + sha-* tags)**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-05-04T17:55:26Z
- **Completed:** 2026-05-04T17:58:00Z
- **Tasks:** 2
- **Files created:** 3

## Accomplishments

- Dockerfile builds a FROM scratch image of 11.46 MB (well under 25 MB target) with CGO_ENABLED=0, -ldflags="-w -s", and ca-certificates.crt copied from the builder stage
- docker-compose.yml ships with AUTH_PROXY: "unconfigured" — `docker compose up` fails immediately with a helpful error; operators must configure before any traffic is served
- CI workflow with three jobs mirroring karaclean: test (CGO_ENABLED=0 -race -count=1) + lint (golangci-lint v2.11) gate the docker job which pushes latest and sha-* tags to ghcr.io

## Docker Image Details

| Property | Value |
|----------|-------|
| Builder base | `golang:1.26-alpine` |
| Final base | `FROM scratch` |
| Files copied to final | `ca-certificates.crt`, `polar-flow-mcp` binary |
| Image size | 12,018,507 bytes (11.46 MB) |
| Fail-closed | `docker run --rm` exits 1 with AUTH_PROXY error (default unconfigured) |

## CI Job Structure

| Job | Trigger | Key config |
|-----|---------|-----------|
| `test` | push/PR to main | `CGO_ENABLED=0 go test -race -count=1 ./...`; `go-version-file: go.mod` |
| `lint` | push/PR to main | `golangci/golangci-lint-action@v9`, `version: v2.11`; `go-version-file: go.mod` |
| `docker` | push/PR to main; `needs: [test, lint]` | builds always; pushes on non-PR events; `latest` + `sha-<short>` tags to `ghcr.io/${{ github.repository }}` |

## docker-compose.yml Environment Variables

| Variable | Default | Required |
|----------|---------|----------|
| `AUTH_PROXY` | `"unconfigured"` | Yes — server refuses to start with this default |
| `PROXY_SHARED_SECRET` | `""` | Yes — shared secret with reverse proxy |
| `ENCRYPTION_KEY` | `""` | Yes — AES-256 key for OAuth tokens |
| `POLAR_CLIENT_ID` | `""` | Yes — Polar OAuth app credential |
| `POLAR_CLIENT_SECRET` | `""` | Yes — Polar OAuth app credential |
| `IDENTITY_HEADER` | (commented out, default `Remote-User`) | No |
| `BIND_ADDRESS` | (commented out, default `127.0.0.1`) | No |
| `DATABASE_PATH` | `"/data/polar.db"` | No |

## Task Commits

1. **Task 1: Multi-stage Dockerfile and docker-compose.yml** — `4759da0` (feat)
2. **Task 2: GitHub Actions CI workflow** — `59193fd` (feat)

**Plan metadata:** (this commit)

## Files Created

- `/home/lm/git/polar-flow-mcp/Dockerfile` — Two-stage build: golang:1.26-alpine → FROM scratch; CGO_ENABLED=0; ca-certificates.crt; binary-only final image
- `/home/lm/git/polar-flow-mcp/docker-compose.yml` — Operator example with AUTH_PROXY=unconfigured fail-closed sentinel; polar-data volume; all env vars documented
- `/home/lm/git/polar-flow-mcp/.github/workflows/ci.yml` — Three jobs: test (CGO_ENABLED=0 -race), lint (golangci-lint v2.11), docker (ghcr.io push, latest + sha-* tags)

## Decisions Made

- FROM scratch chosen over distroless: eliminates shell, package manager, and OS entirely; attack surface is the binary only (T-05-01 mitigation)
- ca-certificates.crt copy from builder is mandatory: FROM scratch has no cert store; without it all HTTPS to Polar API fails with x509 error (Pitfall 7)
- AUTH_PROXY=unconfigured in docker-compose.yml is a deliberate UX contract: a naive `docker compose up` before configuration must fail loudly, not silently serve insecure traffic (T-05-03 mitigation)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required at this stage. Operators will need to configure the docker-compose.yml environment variables when deploying.

## Next Phase Readiness

- Phase 1 complete: all 5 plans done; deployable artifact with CI pipeline in place
- Docker image at ghcr.io/lm/polar-flow-mcp available after first CI run on main branch push
- Phase 2 (OAuth Link Flow + UserInfo) can begin: the container, database, and auth skeleton are ready

---
*Phase: 01-project-foundation-and-security-skeleton*
*Completed: 2026-05-04*
