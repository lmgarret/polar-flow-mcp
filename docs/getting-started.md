# Getting Started

This guide walks you through deploying polar-flow-mcp from scratch — from registering a
Polar developer app to your first Claude tool call.

---

## Prerequisites

Before you start, you need:

- **A Polar account** — the account whose training targets you want to manage.
- **A running reverse proxy** — one of: Authelia, Authentik, oauth2-proxy, Pomerium, or
  Cloudflare Access. The proxy must be able to inject HTTP headers into requests.
- **Docker and Docker Compose** — to run the polar-flow-mcp container.
- **A domain or host** reachable by your reverse proxy (e.g., `polar.example.com`).

---

## Step 1: Register a Polar developer app

You need a Polar OAuth application to obtain a `POLAR_CLIENT_ID` and
`POLAR_CLIENT_SECRET`.

See the [Polar OAuth Setup guide](deployment/polar-oauth-setup.md) for a step-by-step
walkthrough. The key steps are:

1. Visit [https://admin.polaraccesslink.com](https://admin.polaraccesslink.com) and sign
   in with your Polar account.
2. Create a new application.
3. Set the redirect URI to `https://<your-host>/oauth/callback`.
4. Request the `accesslink.read_all` scope.
5. Copy the **Client ID** and **Client Secret** — you will need them in Step 3.

---

## Step 2: Generate secrets

You need two secrets for your deployment:

**Encryption key** (32 bytes, base64-encoded) — used to encrypt Polar OAuth tokens
stored in SQLite:

```bash
openssl rand -base64 32
```

**Proxy shared secret** (hex string) — used to verify that requests arrive from your
trusted reverse proxy and not from a direct HTTP client:

```bash
openssl rand -hex 32
```

Save both values. The encryption key **cannot be changed** after tokens are stored —
if you lose it, users will need to re-authorize via the OAuth flow.

---

## Step 3: Configure Docker Compose

Download the `docker-compose.yml` from the repository and fill in your values:

```yaml
services:
  polar-flow-mcp:
    image: ghcr.io/lmgarret/polar-flow-mcp:latest
    restart: unless-stopped
    volumes:
      - polar-data:/data
    environment:
      AUTH_PROXY: "authelia"                    # or: authentik, oauth2-proxy, etc.
      PROXY_SHARED_SECRET: "<your-hex-secret>"
      ENCRYPTION_KEY: "<your-base64-key>"
      POLAR_CLIENT_ID: "<from-step-1>"
      POLAR_CLIENT_SECRET: "<from-step-1>"
      DATABASE_PATH: "/data/polar.db"

volumes:
  polar-data:
```

The `AUTH_PROXY` value is informational — it tells the server which proxy is in front
of it and appears in the startup log. The server will **refuse to start** if you leave
it as `unconfigured`.

See [Environment Variables reference](reference/env-vars.md) for the full list of
available options including `IDENTITY_HEADER` and `BIND_ADDRESS`.

---

## Step 4: Configure your reverse proxy

Your reverse proxy must inject two headers on every request to polar-flow-mcp:

| Header | Value |
|--------|-------|
| `Remote-User` | Authenticated user identity (e.g., `alice`) |
| `X-Proxy-Secret` | The value of `PROXY_SHARED_SECRET` |

See the [Auth Proxies guide](deployment/auth-proxies.md) for configuration examples for
Authelia, Authentik, oauth2-proxy, Pomerium, and Cloudflare Access.

The `IDENTITY_HEADER` env var lets you change `Remote-User` to whatever header your
proxy injects (some proxies use `X-Forwarded-User` or `X-Auth-Request-User`).

---

## Step 5: Start the server

```bash
docker compose up -d
```

Verify the server is running:

```bash
curl https://<your-host>/healthz
# → ok

curl https://<your-host>/readyz
# → {"checks":[{"name":"auth_proxy","status":"ok"},{"name":"proxy_secret","status":"ok"},{"name":"database","status":"ok"},{"name":"encryption_key","status":"ok"}]}
```

If any check shows `"status":"fail"`, the error message tells you what is wrong. Common
issues:

- `AUTH_PROXY not configured` — you left `AUTH_PROXY: "unconfigured"` in the compose
  file.
- `PROXY_SHARED_SECRET not set` — `PROXY_SHARED_SECRET` is empty.
- `encryption key not loaded or wrong length` — `ENCRYPTION_KEY` is missing or not a
  valid 32-byte base64 string.

---

## Step 6: Link your Polar account

![Polar OAuth login page](images/polar-oauth-flow.png)

Visit `https://<your-host>/oauth/login` in your browser. You will be redirected to
Polar Flow to authorize access.

After authorizing, Polar redirects you back to `/oauth/callback`. The server exchanges
the authorization code for a token, encrypts it, and stores it in SQLite. You only need
to do this once per user — tokens do not expire unless revoked.

If you see an error at this step, check:

- The redirect URI registered in your Polar developer app matches
  `https://<your-host>/oauth/callback` exactly (including the scheme and path).
- Your reverse proxy is injecting the `Remote-User` header before the request reaches
  polar-flow-mcp.

---

## Step 7: Install the polar-coach skill in Claude

See [Usage — Installing the skill](usage.md#installing-the-skill) for both installation
paths (Claude Desktop/Code and Claude.ai).

---

## Step 8: Verify with a tool call

In a Claude conversation with the polar-coach skill active, ask:

> "What Polar account is linked?"

Claude will call `get_user_info`. A successful response looks like:

```
Your Polar account is linked. Polar user ID: 12345678.
```

If Claude reports the server is not connected, verify your MCP server URL and that the
polar-flow-mcp container is reachable from your Claude client.

---

## Next steps

- [Usage guide](usage.md) — all four MCP tools with worked examples
- [Security](security.md) — understand the trust model and encryption design
- [Reference: Environment Variables](reference/env-vars.md) — full configuration
  reference
