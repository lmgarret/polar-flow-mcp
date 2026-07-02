---
title: Design decisions
description: The load-bearing architectural choices behind polar-flow-mcp — single-user, cookies-only persistence, no database, browser-fingerprint TLS, and the request lifecycle.
sidebar:
  order: 2
---

This page explains the load-bearing choices behind polar-flow-mcp — the ones
that shape how it behaves and that shouldn't be revisited without discussion.

## Single-user via env vars

The server logs into exactly one Polar account (`POLAR_EMAIL` /
`POLAR_PASSWORD`). To serve more than one account, run more than one instance
(e.g. one container per user — see
[Deploy with Docker Compose](../../guides/deploy-with-docker-compose/#multiple-accounts)).

The Flow web API requires email + password. Storing per-user passwords is a step
up in risk over OAuth tokens — passwords are often reused and can't be revoked
granularly — so one container per Polar account is the recommended multi-user
pattern rather than a shared multi-tenant server.

## Cookies-only persistence

The password is held in memory only. The server logs in once, persists
`FLOW_SESSION` + `remember-me` to a chmod-600 JSON jar (`COOKIE_JAR_PATH`), and
re-uses them on later runs. Polar's rolling 14-day `remember-me` cookie means an
active server never has to re-prompt.

There is **no database** — no SQLite, no migrations, no encrypted token store.
Just the cookie-jar file. This keeps the image tiny (`FROM scratch`, ~14 MB) and
the operational surface small.

## Generated client, custom transport

Every wire call is type-checked Go generated from the OpenAPI spec by ogen. A
custom `ht.Client` transport wraps ogen's generated client so we can inject the
cookie jar, the required headers, and the 401-retry **without touching generated
code** (which gets overwritten on every regen).

ogen is used because it has the best OpenAPI 3.1 coverage in Go. Its one quirk:
it rejects the 3.1 nullable-union syntax, so the spec is preprocessed to 3.0.3
via `internal/flow/preprocess-spec.py` before generation.

## Browser-fingerprint TLS + HTTP/2

`flow.polar.com` sits behind a CloudFront WAF that inspects JA3/JA4 fingerprints
**and** HTTP/2 framing. Default Go `net/http` gets `403 X-Cache: Error from
cloudfront` on the login redirect. uTLS alone (TLS only) is insufficient. So:

- The **login chain** uses `azuretls` with a Chrome JA3 + HTTP/2 preset.
- The **stdlib `http.Client`** used by ogen for API calls uses a Chrome uTLS
  ClientHello over `http2.Transport`.

## The `X-Requested-With` header

Play's CSRF filter (the Polar backend) whitelists the
`X-Requested-With: XMLHttpRequest` header. The transport adds it on **every
mutation** (any method other than GET), not just `/api/*` calls —
`DELETE /training/target/{id}` sits outside `/api/*` and needs it too. Without
it you get a `403` with an HTML body. An easy footgun.

## Bind-first / deferred login

`flow.New` never logs in. A full login takes seconds (CloudFront WAF + redirect
chain), and blocking startup on it would block the listener from binding, which
races the MCP client's `initialize` call and times it out at 60 s. So the server
binds first and logs in lazily (`EnsureSession` runs from the transport, warmed
up in a background goroutine). Binding first makes the MCP handshake instant.

## Optional inbound OAuth (app-as-Authorization-Server)

Enabled by `OAUTH_PUBLIC_URL` (off by default), the server can act as its **own**
OAuth 2.1 Authorization Server with Dynamic Client Registration — so it can be a
Claude.ai connector *and* work with Claude Code, both of which speak DCR. It
signs and validates its own EdDSA JWT access tokens locally (no database, no
introspection). Browser login on `/authorize` is delegated to a forward-auth
proxy. See [Expose the server securely](../../guides/expose-securely/) and the
[security model](../security-model/) for the trust boundaries.

## The request lifecycle

What the server does on every Polar API request:

1. The ogen-generated client builds the HTTP request.
2. A custom `http.RoundTripper` injects:
   - `X-Requested-With: XMLHttpRequest` on every mutation (Play's CSRF filter
     requires this).
   - A browser-ish `User-Agent` (Polar serves different responses to non-browser
     UAs on adjacent endpoints).
   - The current `FLOW_SESSION` cookie via Go's `net/http/cookiejar`.
3. If Polar responds `401 {"error":"NotAuthenticated"}`, the wrapper runs the
   3-hop silent refresh (`/flowSso/login → /oauth/authorize?continue →
   /flowSso/redirect`). On success it replaces the stale `FLOW_SESSION` cookie on
   the original request and retries once. On failure it falls back to a full
   email/password login.
4. The updated cookie jar is persisted to disk best-effort (failures are logged
   but don't fail the API call).

This is why callers never see a `401` unless the credentials themselves are bad —
the refresh-then-retry is transparent.
