# Contributing

Contributions are welcome! This page covers how to set up your development environment,
run the test suite, and submit a pull request.

The authoritative version of these guidelines is [CONTRIBUTING.md](https://github.com/lmgarret/polar-flow-mcp/blob/main/CONTRIBUTING.md)
in the repository root.

---

## Prerequisites

- **Go 1.26+** — install from [https://go.dev/dl/](https://go.dev/dl/)
- **golangci-lint v2.11** — the linter version pinned for this project

Install golangci-lint to `~/go/bin/` (the `go install` path does not work for v2.x — use the install script):

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b ~/go/bin v2.11.0
```

Verify:

```bash
~/go/bin/golangci-lint --version
# golangci-lint has version v2.11.x
```

---

## Clone and build

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cd polar-flow-mcp
make build
```

The binary is written to `bin/polar-flow-mcp`. The build uses `CGO_ENABLED=0` so no C
toolchain is required.

---

## Run the tests

```bash
make test
```

This runs:

```bash
CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...
```

The `-tags=polartest` flag is required — it enables test-only code that allows
redirecting HTTP requests to test servers. Omitting the flag will cause test failures.

All tests must pass before submitting a PR. The CI pipeline runs the same command.

---

## Run the linter

```bash
make lint
```

This runs:

```bash
~/go/bin/golangci-lint run ./...
```

The linter is configured in `.golangci.yml` at the repo root. Enabled linters include
`gocyclo`, `godot`, `misspell`, `noctx`, and `errcheck` (with type assertion checking).
Fix all lint errors before submitting a PR.

---

## Pre-PR checklist

Before opening a pull request:

- [ ] `make test` passes
- [ ] `make lint` passes with zero errors
- [ ] Documentation updated if you changed behavior or added a feature
- [ ] Commit messages follow conventional commit format (see below)

---

## Commit message format

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]
```

**Types:**

| Type | When to use |
|------|-------------|
| `feat` | New feature or new MCP tool |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `chore` | Maintenance (deps, config) |
| `refactor` | Code restructuring without behavior change |
| `test` | Test additions or changes |
| `ci` | CI workflow changes |

**Examples:**

```
feat(mcp): add list_training_targets tool
fix(oauth): handle expired CSRF state gracefully
docs(security): document key rotation plan
chore(deps): update mcp-go to v0.8.0
```

Conventional commits are used by git-cliff to generate the changelog on release. The
`chore(release)` type is reserved for the automated CHANGELOG commit and must not be
used manually.

---

## Running locally

To run the server locally for development, you need a `.env` file (or export variables)
with at minimum:

```bash
export AUTH_PROXY=dev
export PROXY_SHARED_SECRET=dev-secret
export ENCRYPTION_KEY=$(openssl rand -base64 32)
export POLAR_CLIENT_ID=<your-dev-app-id>
export POLAR_CLIENT_SECRET=<your-dev-app-secret>
export DATABASE_PATH=polar-dev.db
```

Then:

```bash
make build
./bin/polar-flow-mcp
```

The server binds to `127.0.0.1:8080` by default. Test with:

```bash
curl http://127.0.0.1:8080/healthz
```

For OAuth testing, use a tunnel (e.g., `ngrok`) so Polar can redirect back to your
`/oauth/callback` endpoint.

---

## Code style

- Follow standard Go formatting (`gofmt`).
- Keep cyclomatic complexity below 15 per function (`gocyclo` will catch violations).
- End comments with a period (`godot` will catch violations).
- Use `log/slog` for all logging — no `fmt.Println` or `log.Printf`.
- Context keys must use unexported struct types to prevent cross-package collisions.
