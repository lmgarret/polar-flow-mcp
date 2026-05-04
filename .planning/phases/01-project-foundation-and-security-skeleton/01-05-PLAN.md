---
phase: 01-project-foundation-and-security-skeleton
plan: 05
type: execute
wave: 3
depends_on:
  - 01-PLAN-02-config-validation.md
  - 01-PLAN-03-sqlite-crypto.md
files_modified:
  - Dockerfile
  - docker-compose.yml
  - .github/workflows/ci.yml
autonomous: true
requirements:
  - SERV-06
  - SERV-07
  - SERV-08

must_haves:
  truths:
    - "docker build . produces a working image under 25 MB"
    - "docker run with AUTH_PROXY=unconfigured exits non-zero and prints AUTH_PROXY error"
    - "docker-compose.yml ships with AUTH_PROXY=unconfigured so naive docker compose up fails loudly"
    - "CI has three jobs: test, lint, docker — docker needs both test and lint"
    - "CI test job uses CGO_ENABLED=0 and -race flags"
    - "CI docker job builds and pushes to ghcr.io with latest + SHA tags"
  artifacts:
    - path: "Dockerfile"
      provides: "Multi-stage build: golang:1.26-alpine builder → FROM scratch final"
      contains: "FROM scratch"
    - path: "docker-compose.yml"
      provides: "Example compose with AUTH_PROXY=unconfigured and explanatory comments"
      contains: "AUTH_PROXY=unconfigured"
    - path: ".github/workflows/ci.yml"
      provides: "Three-job CI: test + lint → docker"
      contains: "golangci-lint-action"
  key_links:
    - from: "Dockerfile"
      to: "ca-certificates.crt"
      via: "COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/"
      pattern: "ca-certificates.crt"
    - from: ".github/workflows/ci.yml"
      to: "ghcr.io/${{ github.repository }}"
      via: "docker/build-push-action with tags latest and sha-<commit>"
      pattern: "ghcr.io"
---

<objective>
Create the multi-stage Dockerfile (golang:1.26-alpine builder → FROM scratch final), docker-compose.yml that fails loudly with AUTH_PROXY=unconfigured, and the three-job GitHub Actions CI workflow (test + lint → docker) mirroring karaclean exactly.

Purpose: SERV-06..08 complete the deployable artifact and the CI pipeline. The Dockerfile must mirror karaclean (FROM scratch, ca-certificates.crt, CGO_ENABLED=0, -ldflags="-w -s"). docker-compose.yml ships deliberately broken (AUTH_PROXY=unconfigured) to force operator action before any request is served.

Output: `Dockerfile`, `docker-compose.yml`, `.github/workflows/ci.yml`.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md
@.planning/research/STACK.md

@.planning/phases/01-project-foundation-and-security-skeleton/01-01-SUMMARY.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-02-SUMMARY.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Multi-stage Dockerfile and docker-compose.yml</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-13: golang:1.26-alpine → FROM scratch, CGO_ENABLED=0, -ldflags="-w -s", ca-certificates.crt)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/PITFALLS.md (Pitfall 7: scratch image missing TLS certs)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/STACK.md (Docker section — mirror karaclean exactly)
  </read_first>
  <files>
    Dockerfile,
    docker-compose.yml
  </files>
  <action>
Create **Dockerfile** as a two-stage build mirroring karaclean exactly:

```dockerfile
# syntax=docker/dockerfile:1

# Stage 1: Build
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy dependency manifests first for layer caching
COPY go.mod go.sum ./
RUN CGO_ENABLED=0 go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s" \
    -o polar-flow-mcp \
    ./cmd/polar-flow-mcp

# Stage 2: Minimal final image
FROM scratch

# TLS certificates required for outbound HTTPS to Polar API
# Without this, all HTTPS calls fail with x509: certificate signed by unknown authority
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Binary only — no shell, no package manager, no OS
COPY --from=builder /build/polar-flow-mcp /polar-flow-mcp

EXPOSE 8080

ENTRYPOINT ["/polar-flow-mcp"]
```

Key invariants that MUST be present:
- `FROM golang:1.26-alpine` (not 1.25, not latest)
- `FROM scratch` (not alpine, not distroless)
- `CGO_ENABLED=0` on both the `go mod download` and `go build` steps
- `-ldflags="-w -s"` to strip debug info and symbol table
- `COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/` (Pitfall 7)
- No timezone data copy (not needed — UTC timestamps only, per PITFALLS.md)

Create **docker-compose.yml** as the operator example. It must:
1. Use `AUTH_PROXY=unconfigured` deliberately (the server will exit 1 immediately — this is the intended UX contract for operators per SERV-08).
2. Have explanatory comments so operators understand what each env var does.
3. Mount a `polar-data` volume for the SQLite database file.
4. Use the ghcr.io image reference.

```yaml
# polar-flow-mcp docker-compose example
#
# IMPORTANT: This file ships with intentional placeholder values.
# The server will REFUSE TO START until you configure the required variables.
# See https://github.com/lm/polar-flow-mcp/docs/deployment for setup instructions.

services:
  polar-flow-mcp:
    image: ghcr.io/lm/polar-flow-mcp:latest
    restart: unless-stopped
    volumes:
      - polar-data:/data
    environment:
      # REQUIRED: Set to the name of your reverse proxy (authelia, authentik, oauth2-proxy, etc.)
      # The server refuses to start with the default "unconfigured" value.
      AUTH_PROXY: "unconfigured"

      # REQUIRED: A secret shared with your reverse proxy to prevent header spoofing.
      # Generate with: openssl rand -hex 32
      PROXY_SHARED_SECRET: ""

      # REQUIRED: Your AES-256 encryption key for Polar OAuth tokens.
      # Generate with: openssl rand -base64 32
      ENCRYPTION_KEY: ""

      # REQUIRED: Polar OAuth application credentials (register at https://admin.polaraccesslink.com)
      POLAR_CLIENT_ID: ""
      POLAR_CLIENT_SECRET: ""

      # OPTIONAL: The header your reverse proxy injects for user identity (default: Remote-User)
      # IDENTITY_HEADER: "Remote-User"

      # OPTIONAL: Bind address (default: 127.0.0.1 — loopback only)
      # BIND_ADDRESS: "127.0.0.1"

      # OPTIONAL: SQLite database path inside the container (default: polar.db)
      DATABASE_PATH: "/data/polar.db"

volumes:
  polar-data:
```
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && docker build -t polar-flow-mcp:test . && echo "build OK"</automated>
  </verify>
  <acceptance_criteria>
    - `Dockerfile` first non-comment line contains `FROM golang:1.26-alpine`
    - `Dockerfile` contains `FROM scratch`
    - `Dockerfile` contains `CGO_ENABLED=0` in the build RUN step
    - `Dockerfile` contains `-ldflags="-w -s"`
    - `Dockerfile` contains `COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/`
    - `Dockerfile` does NOT contain `FROM alpine`, `FROM debian`, `FROM distroless` in the final stage
    - `docker-compose.yml` contains `AUTH_PROXY: "unconfigured"`
    - `docker-compose.yml` contains `polar-data` volume definition
    - `docker build -t polar-flow-mcp:test .` exits 0
    - `docker image inspect polar-flow-mcp:test --format '{{.Size}}'` output is less than 26214400 (25 MB in bytes)
    - `docker run --rm polar-flow-mcp:test; echo $?` prints a non-zero exit code (fails due to unconfigured defaults)
  </acceptance_criteria>
  <done>Dockerfile builds FROM scratch image under 25 MB with ca-certificates.crt; docker-compose.yml ships with AUTH_PROXY=unconfigured; docker build and size checks pass.</done>
</task>

<task type="auto">
  <name>Task 2: GitHub Actions CI workflow (test + lint → docker)</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-15: golangci-lint v2.11)
    - /var/home/lm/git/polar-flow-mcp/.planning/REQUIREMENTS.md (SERV-07: three jobs mirroring karaclean, ghcr.io push, latest + SHA tags)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/STACK.md (CI section: golangci-lint-action@v9, version v2.11, go-version-file: go.mod)
  </read_first>
  <files>
    .github/workflows/ci.yml
  </files>
  <action>
Create `.github/workflows/ci.yml` with exactly three jobs mirroring karaclean:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Run tests
        run: CGO_ENABLED=0 go test -race -count=1 ./...

  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11

  docker:
    name: Docker
    runs-on: ubuntu-latest
    needs: [test, lint]
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix=sha-,format=short

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

Key invariants:
- `test` job: `CGO_ENABLED=0 go test -race -count=1 ./...` — both flags required (per SERV-07 and CLAUDE.md pre-commit checklist)
- `lint` job: `golangci/golangci-lint-action@v9` with `version: v2.11` exactly (D-15)
- `docker` job: `needs: [test, lint]` — only builds after both pass
- `go-version-file: go.mod` on setup-go in all jobs (mirrors karaclean — auto-updates when toolchain bumped)
- Tags: `latest` on default branch pushes + `sha-<short>` always
- `push: ${{ github.event_name != 'pull_request' }}` — PRs build but don't push
- `permissions: packages: write` on docker job (required for ghcr.io push)
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && cat .github/workflows/ci.yml | grep -c "CGO_ENABLED=0"</automated>
  </verify>
  <acceptance_criteria>
    - `.github/workflows/ci.yml` exists
    - Contains job `test` with `CGO_ENABLED=0 go test -race -count=1 ./...`
    - Contains job `lint` with `golangci/golangci-lint-action@v9` and `version: v2.11`
    - Contains job `docker` with `needs: [test, lint]`
    - `docker` job contains `ghcr.io/${{ github.repository }}`
    - `docker` job contains both `latest` and `sha-` tag configurations
    - `docker` job has `permissions: packages: write`
    - `grep "go-version-file: go.mod" .github/workflows/ci.yml` returns at least 2 matches (test + lint jobs)
    - `grep "CGO_ENABLED=0" .github/workflows/ci.yml` returns exactly 1 match (test job only)
    - YAML is syntactically valid: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` exits 0
  </acceptance_criteria>
  <done>CI workflow with test + lint → docker jobs created; test job uses CGO_ENABLED=0 -race; lint job uses golangci-lint v2.11; docker job builds and pushes to ghcr.io with latest + SHA tags.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| internet→container | FROM scratch exposes no shell, no package manager — attack surface is binary only |
| CI→ghcr.io | GITHUB_TOKEN scoped to packages:write; no long-lived credentials stored |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-05-01 | Elevation of Privilege | FROM scratch attack surface | mitigate | No shell, no OS tools, no package manager in final image; binary-only reduces exploitable surface |
| T-05-02 | Spoofing | Polar API TLS verification | mitigate | `ca-certificates.crt` copied from builder; all outbound HTTPS verified against system roots |
| T-05-03 | Denial of Service | naive docker compose up | mitigate | AUTH_PROXY=unconfigured causes immediate exit 1 — no server starts with default config |
| T-05-04 | Tampering | CI supply chain | accept | actions/checkout@v4, setup-go@v6, docker/build-push-action@v6 are pinned to major versions; SHA pinning deferred to v2 hardening pass |
| T-05-05 | Information Disclosure | GITHUB_TOKEN scope | mitigate | `permissions: packages: write` declared explicitly on docker job only; principle of least privilege |
| T-05-06 | Elevation of Privilege | container runs as root (UID 0) | accept | FROM scratch has no /etc/passwd; process runs as UID 0 by default; noted as future hardening (add USER directive in v2); karaclean parity maintained |
</threat_model>

<verification>
```bash
cd /var/home/lm/git/polar-flow-mcp

# Docker build
docker build -t polar-flow-mcp:test .

# Image size check (must be < 25 MB = 26214400 bytes)
docker image inspect polar-flow-mcp:test --format '{{.Size}}'

# Fail-closed check: exits non-zero with AUTH_PROXY=unconfigured
docker run --rm -e AUTH_PROXY=unconfigured polar-flow-mcp:test; echo "exit: $?"

# ca-certificates present in image
docker run --rm --entrypoint="" polar-flow-mcp:test ls /etc/ssl/certs/ca-certificates.crt 2>/dev/null || echo "no shell in scratch — expected"

# CI workflow YAML validity
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML OK"

# Verify CI job structure
grep -E "^  (test|lint|docker):" .github/workflows/ci.yml
grep "needs: \[test, lint\]" .github/workflows/ci.yml
grep "version: v2.11" .github/workflows/ci.yml
grep "CGO_ENABLED=0" .github/workflows/ci.yml

# docker-compose.yml has unconfigured sentinel
grep "AUTH_PROXY.*unconfigured" docker-compose.yml
```
</verification>

<success_criteria>
- `docker build .` exits 0
- Final image size is under 25 MB
- `docker run --rm -e AUTH_PROXY=unconfigured <image>` exits non-zero with an error message containing "AUTH_PROXY"
- `docker-compose.yml` contains `AUTH_PROXY: "unconfigured"` — naive `docker compose up` fails loudly
- CI workflow has exactly 3 jobs: test, lint, docker; docker needs both
- CI test job uses `CGO_ENABLED=0 go test -race -count=1 ./...`
- CI lint job uses `golangci/golangci-lint-action@v9` with `version: v2.11`
- CI docker job pushes to `ghcr.io/${{ github.repository }}` with `latest` and `sha-*` tags
</success_criteria>

<output>
After completion, create `.planning/phases/01-project-foundation-and-security-skeleton/01-05-SUMMARY.md` with:
- Docker image size (bytes and MB from docker inspect)
- Dockerfile stage summary (builder base, final base, files copied)
- CI job names and their trigger conditions
- docker-compose.yml environment variable list
- Confirmation that AUTH_PROXY=unconfigured causes non-zero exit
</output>
