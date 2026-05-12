# Environment Variables

All configuration is provided through environment variables. The server validates all
required variables at startup and exits with a non-zero code if any are missing or
invalid. There is no configuration file — the `docker-compose.yml` environment block
is the authoritative source.

---

## Required variables

These variables must be set. The server will **refuse to start** if any are absent,
empty, or set to their placeholder defaults.

### `AUTH_PROXY`

| Field | Value |
|-------|-------|
| Required | Yes |
| Default | `unconfigured` (server refuses to start) |
| Example | `authelia`, `authentik`, `oauth2-proxy` |

The name of the reverse proxy in front of polar-flow-mcp. This value is informational —
it is printed in the startup banner and log output to make it easy to confirm the correct
proxy is configured.

The server refuses to start if `AUTH_PROXY` is `unconfigured` or empty. The
`docker-compose.yml` ships with `AUTH_PROXY: "unconfigured"` as a deliberate UX
contract: a copy-paste deployment that skips configuration fails loudly rather than
silently running in an insecure state.

See [Security — Fail-closed rationale](../security.md#fail-closed-rationale) for the
design reasoning.

---

### `PROXY_SHARED_SECRET`

| Field | Value |
|-------|-------|
| Required | Yes |
| Default | (none) |
| How to generate | `openssl rand -hex 32` |
| Example | `a3f8c2...` (64 hex characters) |

A secret shared between your reverse proxy and polar-flow-mcp. The server verifies this
secret on **every request** using `subtle.ConstantTimeCompare` before reading the user
identity header. This prevents a direct HTTP client from bypassing the proxy and injecting
arbitrary identity headers.

Your reverse proxy must inject this value as the `X-Proxy-Secret` header on all
forwarded requests. See [Auth Proxies](../deployment/auth-proxies.md) for proxy-specific
configuration examples.

---

### `ENCRYPTION_KEY`

| Field | Value |
|-------|-------|
| Required | Yes |
| Format | 32-byte key, base64-encoded (standard or URL-safe) |
| How to generate | `openssl rand -base64 32` |
| Example | `K8zP...=` (44 base64 characters) |

The AES-256-GCM encryption key used to encrypt Polar OAuth tokens before storing them
in SQLite. The key must decode to exactly 32 bytes.

**Important:** This key cannot be rotated in v1. If you change `ENCRYPTION_KEY` after
tokens have been stored, all users will need to re-authorize via the OAuth flow. Store
this key securely — losing it means losing all stored tokens.

See [Security — Encryption at rest](../security.md#encryption-at-rest) for details on
the encryption scheme.

---

### `POLAR_CLIENT_ID`

| Field | Value |
|-------|-------|
| Required | Yes |
| Source | Polar developer console at [admin.polaraccesslink.com](https://admin.polaraccesslink.com) |

The OAuth client ID for your registered Polar developer application.
See [Polar OAuth Setup](../deployment/polar-oauth-setup.md) for registration instructions.

---

### `POLAR_CLIENT_SECRET`

| Field | Value |
|-------|-------|
| Required | Yes |
| Source | Polar developer console at [admin.polaraccesslink.com](https://admin.polaraccesslink.com) |

The OAuth client secret for your registered Polar developer application. Treat this value
like a password — do not commit it to version control.

---

## Optional variables

These variables have defaults that work for most deployments. Override them only if you
have a specific reason.

### `KEY_PROVIDER`

| Field | Value |
|-------|-------|
| Required | No |
| Default | `env` |
| Options | `env`, `file` |

Controls where the encryption key is loaded from.

- `env` (default): loads the key from `ENCRYPTION_KEY` environment variable.
- `file`: loads the key from the file path specified in `ENCRYPTION_KEY_FILE`.

The `file` provider is useful when you want to manage the key as a Docker secret or
Kubernetes secret mounted into the container filesystem.

---

### `ENCRYPTION_KEY_FILE`

| Field | Value |
|-------|-------|
| Required | Only when `KEY_PROVIDER=file` |
| Default | (none) |
| Example | `/run/secrets/encryption_key` |

Path to a file containing the base64-encoded AES-256-GCM key. The file should contain
a single line with the base64-encoded 32-byte key (same format as `ENCRYPTION_KEY`).

---

### `IDENTITY_HEADER`

| Field | Value |
|-------|-------|
| Required | No |
| Default | `Remote-User` |
| Example | `X-Forwarded-User`, `X-Auth-Request-User` |

The HTTP header your reverse proxy injects to identify the authenticated user. Different
proxies use different header names:

| Proxy | Default header |
|-------|----------------|
| Authelia | `Remote-User` |
| Authentik | `X-authentik-username` or `Remote-User` (configurable) |
| oauth2-proxy | `X-Auth-Request-User` |
| Pomerium | `X-Pomerium-Claim-Email` or custom |
| Cloudflare Access | `Cf-Access-Authenticated-User-Email` |

Set `IDENTITY_HEADER` to match whatever header your proxy injects. The value of this
header becomes the user's identity key in the database.

---

### `BIND_ADDRESS`

| Field | Value |
|-------|-------|
| Required | No |
| Default | `127.0.0.1` |
| Example | `0.0.0.0` (bind all interfaces) |

The IP address the HTTP server listens on. Defaults to loopback (`127.0.0.1`) so the
server is not directly reachable without going through the reverse proxy.

If you set `BIND_ADDRESS` to a non-loopback address, the server emits a warning log at
startup:

```
WARN: BIND_ADDRESS is not localhost — ensure PROXY_SHARED_SECRET is set and the
      server is not directly reachable without the reverse proxy
```

This warning is a reminder that a non-loopback bind address increases the risk of
bypass if the proxy is misconfigured.

The server always listens on port 8080 (not configurable in v1).

---

### `DATABASE_PATH`

| Field | Value |
|-------|-------|
| Required | No |
| Default | `polar.db` (relative to working directory) |
| Example | `/data/polar.db` |

Path to the SQLite database file. In Docker deployments, always set this to a path
inside a named volume (e.g., `/data/polar.db`) so the database persists across container
restarts.

---

## Summary table

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AUTH_PROXY` | Yes | `unconfigured` (fails) | Reverse proxy name |
| `PROXY_SHARED_SECRET` | Yes | — | Shared secret for header verification |
| `ENCRYPTION_KEY` | Yes | — | AES-256-GCM key (base64, 32 bytes) |
| `POLAR_CLIENT_ID` | Yes | — | Polar OAuth client ID |
| `POLAR_CLIENT_SECRET` | Yes | — | Polar OAuth client secret |
| `KEY_PROVIDER` | No | `env` | Key source: `env` or `file` |
| `ENCRYPTION_KEY_FILE` | If `KEY_PROVIDER=file` | — | Path to key file |
| `IDENTITY_HEADER` | No | `Remote-User` | User identity header name |
| `BIND_ADDRESS` | No | `127.0.0.1` | HTTP listen address |
| `DATABASE_PATH` | No | `polar.db` | SQLite database file path |
