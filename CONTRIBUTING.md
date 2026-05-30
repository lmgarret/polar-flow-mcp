# Contributing to polar-flow-mcp

Thank you for your interest in contributing! All contributions are welcome — bug reports, feature requests, documentation improvements, and code changes.

## Development setup

Requirements:
- Go 1.26+
- golangci-lint v2.11 installed to `~/go/bin/golangci-lint`

Install golangci-lint:
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b ~/go/bin v2.11.0
```

Build and test:
```bash
make build
make test
make lint
make clean
```

- `make test` runs `CGO_ENABLED=0 go test -count=1 ./...`
- `make lint` runs `~/go/bin/golangci-lint run ./...`

## Workflow

1. Fork the repository and create a feature branch.
2. Make your changes with atomic commits.
3. Use conventional commit messages: `feat`, `fix`, `docs`, `chore`, `refactor`.
4. Open a pull request against `main`.

## Pre-commit checklist

Before every commit:

1. **Lint**: `make lint` — fix all lint errors before committing.
2. **Tests**: `make test` — fix all failures before committing.
3. **Documentation**: Update docs when features are added or modified. Do not defer.

## Code of conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating you agree to uphold it.

## Reporting security issues

Please do **not** file a public issue for security vulnerabilities. See [SECURITY.md](SECURITY.md) for the private reporting process.
