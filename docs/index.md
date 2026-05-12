# polar-flow-mcp

A multi-user MCP (Model Context Protocol) server that wraps the Polar AccessLink API,
letting you create and manage Polar Flow training targets directly from Claude
conversations — zero UI navigation required.

> **"Create a 5×1km threshold session for Thursday"** → structured workout appears in
> your Polar Flow calendar, ready to sync to your watch.

![Claude using create_training_target](images/claude-tool-call.png)

---

## What it does

polar-flow-mcp bridges Claude and the Polar Flow training platform. Once deployed and
linked to your Polar account, you can:

- **Create structured workouts** — warmup, interval repeats with target HR zones,
  cooldown — using plain-language instructions to Claude.
- **List upcoming training targets** — see what you have planned in Polar Flow for the
  next 30 days.
- **Delete training targets** — remove sessions created in error or no longer needed.
- **Confirm account linking** — check which Polar account is currently connected.

All four operations are exposed as MCP tools via the bundled `polar-coach` skill.

---

## Key features

- **Multi-user** — each user authenticated by your reverse proxy gets their own isolated
  Polar token. A single server instance can serve your entire homelab team.
- **Fail-closed by design** — the server refuses to start unless a valid reverse-proxy
  name, shared secret, encryption key, and database are all configured. Silent
  misconfiguration is not possible.
- **Tokens encrypted at rest** — Polar OAuth tokens are stored as AES-256-GCM
  ciphertext (12-byte nonce prepended) in SQLite. A stolen database file without the
  encryption key is useless.
- **No built-in authentication** — auth is fully delegated to your existing reverse proxy
  (Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access). This avoids
  duplicating auth logic and lets you reuse your existing SSO infrastructure.
- **Self-hostable** — designed for homelabs and small teams. A single Docker container
  with a named volume is all you need.

---

## Tech stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go 1.26 | Fast, statically linked, CGO disabled |
| MCP transport | StreamableHTTP (`mcp-go`) | Per-request context, proxy-friendly |
| Database | SQLite (`modernc.org/sqlite`) | Pure Go, zero deployment dependencies |
| Encryption | AES-256-GCM (stdlib) | Standard at-rest protection |
| Auth | Reverse-proxy contract | Reuse existing SSO, no auth bugs in this server |
| Image | `FROM scratch` (~12 MB) | Minimal attack surface |

---

## How the auth model works

polar-flow-mcp does not authenticate users itself. It relies on a **trusted reverse
proxy** (Authelia, Authentik, etc.) to:

1. Authenticate the user.
2. Inject their identity into an HTTP header (default: `Remote-User`).
3. Sign each request with a shared secret (`X-Proxy-Secret` header).

The server verifies the shared secret using `subtle.ConstantTimeCompare` on **every
request**, before reading the identity header. This ordering is non-negotiable — it
prevents a timing oracle attack on the secret. See [Security](security.md) for full
details.

---

## Getting started

If you are deploying for the first time:

1. [Register a Polar developer app](deployment/polar-oauth-setup.md)
2. Follow the [Getting Started guide](getting-started.md) for the full setup walkthrough
3. Configure your [reverse proxy](deployment/auth-proxies.md)
4. Install the [polar-coach skill](usage.md#installing-the-skill) in Claude
