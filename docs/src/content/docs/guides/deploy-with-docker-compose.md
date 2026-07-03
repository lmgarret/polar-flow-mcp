---
title: Deploy with Docker Compose
description: Run polar-flow-mcp as a container with cookies persisted on a named volume, plus the multi-account pattern.
sidebar:
  order: 2
---

The repository ships a `docker-compose.yml` configured for the common case:
one container, one Polar account, cookies persisted on a named volume.

## Quickstart

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cd polar-flow-mcp
$EDITOR docker-compose.yml   # fill in POLAR_EMAIL + POLAR_PASSWORD
docker compose up -d
docker compose logs -f
```

On the first start you should see something like this. Login is deferred so the
listener binds immediately (the MCP handshake is never blocked behind the
login), then a background warm-up runs the full login:

```text
level=INFO msg="polar-flow-mcp starting" transport=http polar_account=…
level=INFO msg="flow: no session yet — login deferred to first request"
level=INFO msg="server listening" addr=0.0.0.0:8080
level=INFO msg="flow: no session — running full login"
```

On subsequent starts those login lines are replaced by a single
`flow: reusing persisted session from cookie jar` — the server re-uses the
persisted cookie jar and skips the password step.

## Volumes

The compose file mounts a named volume `polar-data` at `/data`. Inside, the
server writes:

| Path | Contents | Mode |
|------|----------|------|
| `/data/polar-cookies.json` | Session cookies (`FLOW_SESSION`, `remember-me`). | `0600` |

The volume persists across container recreations — the server will not need
to re-prompt for the password on a routine `docker compose up -d` cycle.

If you delete the volume, the next start performs a fresh login chain using
the env-var credentials.

## Multiple accounts

This server is single-user by design. To serve multiple Polar accounts,
run multiple compose stacks — copy the `polar-flow-mcp` service, give it a
unique container name and a unique published port, and use separate volumes:

```yaml
services:
  polar-flow-alice:
    image: ghcr.io/lmgarret/polar-flow-mcp:latest
    ports: ["127.0.0.1:8080:8080"]
    volumes: [alice-data:/data]
    environment:
      POLAR_EMAIL: "alice@example.com"
      POLAR_PASSWORD: "${ALICE_POLAR_PASSWORD}"
      COOKIE_JAR_PATH: "/data/polar-cookies.json"

  polar-flow-bob:
    image: ghcr.io/lmgarret/polar-flow-mcp:latest
    ports: ["127.0.0.1:8081:8080"]
    volumes: [bob-data:/data]
    environment:
      POLAR_EMAIL: "bob@example.com"
      POLAR_PASSWORD: "${BOB_POLAR_PASSWORD}"
      COOKIE_JAR_PATH: "/data/polar-cookies.json"

volumes:
  alice-data:
  bob-data:
```

Each MCP client points at the appropriate port. See
[Why single-user?](/explanation/design-decisions/#single-user-via-env-vars)
for the reasoning.

## Security defaults

- The compose file publishes port 8080 on **`127.0.0.1` only** by default.
  With `OAUTH_PUBLIC_URL` unset the server has no inbound authentication, so a
  public bind would be a footgun.
- The server logs a warning at startup if `BIND_ADDRESS` is not localhost **and**
  OAuth is disabled.

Two ways to reach it remotely:

- **Private/local** — keep it on `127.0.0.1` and reach it over a trusted-network
  barrier (a Tailscale interface, a VPN, or an SSH tunnel).
- **Public (Claude.ai web/mobile + Claude Code)** — set `OAUTH_PUBLIC_URL` to
  turn the server into its own OAuth 2.1 Authorization Server (+
  `OAUTH_ALLOWED_EMAIL` + `OAUTH_TRUSTED_PROXIES`) and front it with a TLS reverse
  proxy. See [Expose the server securely](/guides/expose-securely/) for the full Caddy +
  Authelia walkthrough.

## Image

Pulled from `ghcr.io/lmgarret/polar-flow-mcp:latest` — a `FROM scratch`
image, ~14 MB, no shell, no package manager. Pin to a SHA tag in production.
