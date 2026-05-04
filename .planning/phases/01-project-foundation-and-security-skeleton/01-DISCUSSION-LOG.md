# Phase 1: Project Foundation and Security Skeleton - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-04
**Phase:** 1-Project Foundation and Security Skeleton
**Areas discussed:** Identity injection method, Test scope for Phase 1

---

## Identity Injection Method

| Option | Description | Selected |
|--------|-------------|----------|
| Auth middleware only | Extract identity in shared-secret middleware, stash via context.WithValue. Guaranteed per-request regardless of mcp-go internals. | ✓ |
| WithHTTPContextFunc (if verified per-request) | Use mcp-go's native hook — cleaner if per-request, but requires verification before trusting. | |
| Both: middleware + WithHTTPContextFunc | Set in middleware (authoritative), WithHTTPContextFunc as passthrough. Defensive but noisy. | |

**User's choice:** Auth middleware only
**Notes:** WithHTTPContextFunc's per-request vs per-session behavior when mounted as http.Handler is unverified. Middleware is the safe, guaranteed approach.

---

### Identity Header Default

| Option | Description | Selected |
|--------|-------------|----------|
| Remote-User | Standard header set by Authelia, Authentik, oauth2-proxy. Most homelab proxies default to this. | ✓ |
| X-Remote-User | Prefixed variant, less common as default. | |
| Configurable, no default | IDENTITY_HEADER required, no implicit default. | |

**User's choice:** Remote-User (configurable via IDENTITY_HEADER env var)

---

### Missing Identity Header Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Return 403, log a warning | Fail closed — no identity means no service. Logged as probable misconfiguration. | ✓ |
| Return 401 | Semantically "unauthenticated". | |
| Return 500 | Treat as internal error. Misleads on root cause. | |

**User's choice:** 403 with warning log

---

## Test Scope for Phase 1

| Option | Description | Selected |
|--------|-------------|----------|
| Unit + server startup integration | Unit tests for crypto + config, plus httptest integration tests for fail-closed behavior, /healthz, /readyz. | ✓ |
| Unit tests only | Crypto and config validation only — no httptest server. | |
| Smoke tests only | Just enough to compile. Real tests in later phases. | |

**User's choice:** Unit + server startup integration

---

### Nonce Uniqueness Test

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — nonce uniqueness test | 1000 encryptions → 1000 distinct nonces. Validates the critical immutable-schema invariant. | ✓ |
| No — trust crypto/rand | CSPRNG, near-zero probability of collision. | |

**User's choice:** Yes — include nonce uniqueness test

---

### readyz Dual Pool Check

| Option | Description | Selected |
|--------|-------------|----------|
| Both pools | Verify write and read pools are both responsive. Wedged write pool + healthy read pool = 503. | ✓ |
| One DB ping is enough | Both pools hit the same file; one ping is sufficient. | |

**User's choice:** Both pools verified in readyz

---

## Claude's Discretion

- **KeyProvider key caching**: Whether env/file provider caches key on first read or re-reads each call — planner decides. Either is correct for v1 single-key scope.
- **Startup error format**: Plain stderr vs structured slog.Error before os.Exit(1) — planner decides. Slog suggested for consistency.

## Deferred Ideas

None — discussion stayed within Phase 1 scope.
