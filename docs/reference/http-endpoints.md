# HTTP Endpoints

polar-flow-mcp exposes a small, focused set of HTTP endpoints. All endpoints serve the
same port (8080 by default, controlled by `BIND_ADDRESS`).

---

## Authentication model

Two endpoints (`/healthz` and `/readyz`) are intentionally public — they need no
authentication because they expose no user data and are used by load balancers and
monitoring tools.

All other endpoints require both:

1. **`X-Proxy-Secret` header** — verified against `PROXY_SHARED_SECRET` using
   `subtle.ConstantTimeCompare`. This check runs **before** the identity header is
   read, preventing a timing oracle on the secret.
2. **Identity header** (default `Remote-User`, configurable via `IDENTITY_HEADER`) —
   the user's identity, injected by the trusted reverse proxy.

A request that passes the secret check but lacks the identity header is rejected with
`403 Forbidden`.

---

## Endpoints

### `GET /healthz`

| Field | Value |
|-------|-------|
| Auth | None |
| Purpose | Liveness probe — confirms the HTTP server is accepting connections |
| Success | `200 OK`, body `ok` |

```bash
curl https://<your-host>/healthz
# ok
```

Use this endpoint for Docker health checks and reverse-proxy liveness probes. It returns
`200` as long as the HTTP server process is running, regardless of database or
configuration state.

---

### `GET /readyz`

| Field | Value |
|-------|-------|
| Auth | None |
| Purpose | Readiness probe — confirms the server is fully configured and operational |
| Success | `200 OK`, JSON body with all checks passing |
| Failure | `503 Service Unavailable`, JSON body listing the failing check(s) |

```bash
curl https://<your-host>/readyz
```

**Success response:**

```json
{
  "checks": [
    {"name": "auth_proxy", "status": "ok"},
    {"name": "proxy_secret", "status": "ok"},
    {"name": "database", "status": "ok"},
    {"name": "encryption_key", "status": "ok"}
  ]
}
```

**Failure response** (example: `AUTH_PROXY` not configured):

```json
{
  "checks": [
    {"name": "auth_proxy", "status": "fail", "error": "AUTH_PROXY not configured"},
    {"name": "proxy_secret", "status": "ok"},
    {"name": "database", "status": "ok"},
    {"name": "encryption_key", "status": "ok"}
  ]
}
```

The four checks are:

| Check name | What it verifies |
|------------|-----------------|
| `auth_proxy` | `AUTH_PROXY` is set and not `unconfigured` |
| `proxy_secret` | `PROXY_SHARED_SECRET` is non-empty |
| `database` | SQLite read pool and write pool respond to ping |
| `encryption_key` | Encryption key is loaded and exactly 32 bytes |

Use `/readyz` for startup probes and startup-blocking checks. Do not use it as a
liveness probe — a database hiccup would cause a container restart.

---

### `GET /oauth/login`

| Field | Value |
|-------|-------|
| Auth | Proxy auth required (`X-Proxy-Secret` + identity header) |
| Purpose | Initiate the Polar OAuth authorization flow |
| Success | `302 Found` — redirects to `https://flow.polar.com/oauth2/authorization?...` |
| Error | `403 Forbidden` if auth check fails |

The server generates a CSRF state token, stores it in SQLite with an expiry, and
redirects the user's browser to the Polar authorization page. The state token is
associated with the user's identity so it can be validated on callback.

Users must visit this URL once to link their Polar account. After a successful OAuth
flow, they do not need to visit it again (Polar tokens do not expire unless revoked).

---

### `GET /oauth/callback`

| Field | Value |
|-------|-------|
| Auth | Proxy auth required (`X-Proxy-Secret` + identity header) |
| Purpose | Handle the OAuth authorization code returned by Polar |
| Success | Exchanges code for token, encrypts and stores token, returns `200 OK` with confirmation |
| Error | `400 Bad Request` for invalid/expired state or missing parameters; `403` for auth failure |

Polar redirects the user here after they authorize access. The server:

1. Verifies the `state` parameter against the stored CSRF state (atomic consume-on-first-use via `DELETE ... RETURNING`).
2. Exchanges the authorization code for an access token using the Polar token endpoint.
3. Registers the user with the Polar AccessLink API if not already registered.
4. Encrypts the token with AES-256-GCM and stores the `nonce || ciphertext` BLOB in the `polar_tokens` table.

The state token is single-use: once consumed by the callback, it cannot be replayed.
Expired state tokens are rejected.

---

### `POST /mcp` and `POST /mcp/`

| Field | Value |
|-------|-------|
| Auth | Proxy auth required (`X-Proxy-Secret` + identity header) |
| Purpose | StreamableHTTP MCP transport endpoint for Claude tool calls |
| Success | `200 OK`, MCP protocol response body |
| Error | `403 Forbidden` for auth failure; MCP error response for tool errors |

Both `/mcp` and `/mcp/` are registered to handle Go's `http.ServeMux` prefix matching
behavior. Claude clients should use `https://<your-host>/mcp` as the MCP server URL.

Tool calls arrive as HTTP POST requests with a JSON body following the MCP protocol.
The server extracts the user identity from the request context (injected by auth
middleware) and passes it to each tool handler. Tool handlers use this identity to look
up the user's Polar token in the database.

Available tools:

| Tool | Description |
|------|-------------|
| `get_user_info` | Return the linked Polar user ID |
| `create_training_target` | Create a structured workout in Polar Flow |
| `list_training_targets` | List upcoming training targets |
| `delete_training_target` | Delete a training target by ID |

See [MCP Tools reference](mcp-tools.md) for input/output schemas.
