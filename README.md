# polar-flow-mcp

Single-user MCP server that drives the **reverse-engineered Polar Flow web API**
(`flow.polar.com`) from Claude — including creating and deleting training targets,
which the official AccessLink API does not allow.

[![CI](https://img.shields.io/github/actions/workflow/status/lmgarret/polar-flow-mcp/ci.yml?branch=main&label=CI)](https://github.com/lmgarret/polar-flow-mcp/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/lmgarret/polar-flow-mcp)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/lmgarret/polar-flow-mcp)](https://github.com/lmgarret/polar-flow-mcp/releases/latest)

> **Not the official Polar AccessLink API.** The web API is reverse-engineered
> from browser traffic. Use a test account where possible. See `internal/flow/README.md`.

📖 **Full documentation:** <https://lmgarret.github.io/polar-flow-mcp/> — tutorials,
how-to guides, reference, and explanation (built with [Astro Starlight](https://starlight.astro.build/)).

## Quickstart

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cd polar-flow-mcp
cp .env.example .env
# Edit .env — set POLAR_EMAIL and POLAR_PASSWORD
go run ./cmd/polar-flow-mcp
```

The server binds its listener immediately and logs into Polar Flow lazily: on a
cold start the login runs in a background warm-up (and again on the first request
if needed), so the MCP handshake is never blocked behind the login round-trip.
The session cookies are persisted to `./polar-cookies.json` (chmod 600) and
re-used on subsequent runs. The password is held in memory only.

> If you connect over HTTP (`"type": "http"` in your MCP client config), make
> sure the server is already running and reachable at the URL before the client
> connects — start the binary or `docker run` it first. The client expects a
> live endpoint; it does not launch the process for you.

## Running with Docker

Pre-built `linux/amd64` images are published to GHCR:

| Tag | Points to |
|-----|-----------|
| `latest` | Newest tagged release (recommended) |
| `X.Y.Z`, `X.Y`, `X` | Semver ladder — pin as tightly as you like |
| `edge` | Rolling build of `main` |
| `sha-<short>` | Exact commit |

Tagged releases (`vX.Y.Z`) also get AI-summarised release notes on the
[Releases page](https://github.com/lmgarret/polar-flow-mcp/releases).

### HTTP transport (recommended for persistent servers)

```bash
# Put credentials in a chmod-600 file, not on the command line
echo "POLAR_EMAIL=you@example.com" | sudo tee /etc/polar-flow/secrets.env
echo "POLAR_PASSWORD=yourpassword" | sudo tee -a /etc/polar-flow/secrets.env
sudo chmod 600 /etc/polar-flow/secrets.env

docker run -d \
  -v /etc/polar-flow/secrets.env:/.env:ro \
  -e TRANSPORT=http \
  -e BIND_ADDRESS=0.0.0.0 \
  -e COOKIE_JAR_PATH=/data/cookies.json \
  -v polar-cookies:/data \
  -p 127.0.0.1:8080:8080 \
  ghcr.io/lmgarret/polar-flow-mcp:latest
```

> `BIND_ADDRESS=0.0.0.0` is required — the default `127.0.0.1` is unreachable from outside the container.

Then point your MCP client at `http://127.0.0.1:8080/mcp`.

### stdio transport (Claude Desktop / Claude Code)

Pass `docker run` as the MCP command so Claude spawns the container itself:

```json
{
  "mcpServers": {
    "polar-flow-mcp": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-e", "POLAR_EMAIL=you@example.com",
        "-e", "POLAR_PASSWORD=yourpassword",
        "-e", "TRANSPORT=stdio",
        "-e", "COOKIE_JAR_PATH=/data/cookies.json",
        "-v", "polar-cookies:/data",
        "ghcr.io/lmgarret/polar-flow-mcp:latest"
      ]
    }
  }
}
```

Two things are non-negotiable for the stdio spawn, and both must be present:

- **`-i`** on `docker run` — without it the container's stdin is not attached, so
  the client's JSON-RPC messages never reach the process.
- **`TRANSPORT=stdio`** — the default is `http`, which makes the server listen on
  a port and ignore stdin entirely.

Also use `--rm` (not `--name`/`--restart`): the client spawns a fresh container
per launch, and a leftover named container makes the next launch fail with
"name already in use".

> **Symptom of getting this wrong:** the client logs `initialize` and then, ~60s
> later, `Request timed out` / `transport closed`, while the server's own logs
> show `msg="polar-flow-mcp starting" transport=http`. That mismatch — the client
> speaking stdio to an HTTP-mode server (or `docker run` missing `-i`) — means the
> handshake never arrives. Add `-i` and `TRANSPORT=stdio`. The HTTP access log
> (`msg="http: request" rpc_method=initialize`) only appears when a request
> actually reaches the HTTP listener, so its absence confirms a stdio/http mixup.

### Docker Compose (Portainer / persistent server)

Credentials are loaded from a secrets file bind-mounted into the container so
they never appear in `docker inspect`, Portainer's UI, or the environment block.

**1. Create the secrets file on the host (once):**

```bash
sudo mkdir -p /etc/polar-flow
sudo tee /etc/polar-flow/secrets.env <<'EOF'
POLAR_EMAIL=you@example.com
POLAR_PASSWORD=yourpassword
EOF
sudo chmod 600 /etc/polar-flow/secrets.env
```

**2. Deploy with the bundled `docker-compose.yml`:**

```yaml
services:
  polar-flow-mcp:
    image: ghcr.io/lmgarret/polar-flow-mcp:latest
    volumes:
      - /etc/polar-flow/secrets.env:/.env:ro   # secrets, never in env block
      - polar-data:/data
    environment:
      COOKIE_JAR_PATH: /data/polar-cookies.json
      TRANSPORT: http
      BIND_ADDRESS: "0.0.0.0"
    ports:
      - "127.0.0.1:8080:8080"
    restart: unless-stopped

volumes:
  polar-data:
```

The server reads `/.env` on startup via `godotenv` — credentials are in memory only and never written to disk by the server itself.

## Tools

| Tool | Description |
|------|-------------|
| `get_user_info` | Linked Polar account identity + profile basics |
| `list_sports` | Full Polar sport-id → name catalogue (the `sport_id` values for targets/sessions) |
| `create_training_target` | Schedule a target (warmup / repeat / cooldown phases) |
| `list_training_targets` | Targets in a date range |
| `delete_training_target` | Delete a target by ID |
| `get_training_target` | Read one target by ID |
| `update_training_target` | Full-replace edit of one target by ID |
| `get_calendar_events` | Raw calendar events in a date range |
| `get_calendar_week_summary` | Per-ISO-week totals strip (≤45-day range) |
| `list_training_sessions` | Completed sessions in a date range |
| `get_training_session_summary` | Summary view of one session |
| `get_training_session_details` | Lap and sample detail of one session |
| `get_progress_summary` | Aggregated training totals over a range |
| `create_training_session` | Log a manually-entered completed session (writes real data — coach must only call on explicit user request) |

`create_training_session` and `delete_training_target` ask the user to confirm
before they run, on hosts that support MCP elicitation. Where the client cannot
be asked, the call proceeds unconfirmed — see
[User confirmation on writes](https://lmgarret.github.io/polar-flow-mcp/reference/mcp-tools/#user-confirmation-on-writes).

## Configuration

See [`.env.example`](.env.example). Required:

- `POLAR_EMAIL`, `POLAR_PASSWORD` — credentials for the Polar Flow account

Optional:

- `COOKIE_JAR_PATH` — where to persist session cookies (default `./polar-cookies.json`)
- `TRANSPORT` — `stdio` or `http` (default `http`)
- `BIND_ADDRESS`, `PORT` — HTTP listen address
- `LOG_LEVEL`, `LOG_FILE`
- `MCP_API_KEY` — enable inbound auth with a fixed pre-shared key (see below)
- `OAUTH_PUBLIC_URL` (+ `OAUTH_ALLOWED_EMAIL`, `OAUTH_TRUSTED_PROXIES`) — enable inbound OAuth (see below)

## Exposing to the internet (Claude.ai)

By default the HTTP transport has **no inbound auth** — run it on localhost or a
trusted network. Two mutually-exclusive mechanisms secure a public deployment.

### Static API key (simplest)

Set `MCP_API_KEY` to a secret of at least 24 characters (`openssl rand -base64 32`)
and every `/mcp` request must carry it as `Authorization: Bearer <key>`. Nothing
else to run — no issuer, no signing key, no forward-auth proxy. Works with
Claude Code:

```bash
claude mcp add --transport http polar-flow https://polar.example.com/mcp \
  --header "Authorization: Bearer $MCP_API_KEY"
```

The trade-off: one long-lived secret shared by every caller, no per-client
identity, and revoking it means restarting with a new key. Serve it over TLS.
Claude.ai's connector UI expects a full OAuth flow, so use OAuth for that.

### OAuth 2.1 (Claude.ai connectors)

Set `OAUTH_PUBLIC_URL` to turn the server into its **own OAuth 2.1
Authorization Server**: clients self-register via Dynamic Client Registration
(nothing to paste), browser login on the consent step is delegated to a
forward-auth proxy (Authelia), and an email allowlist decides who may connect.
Access tokens are short-lived EdDSA JWTs the server signs and validates itself —
no database. Because it provides DCR, **Claude Code (CLI) works over the same
public URL**.

See **[Expose the server securely (Caddy + Authelia + Claude.ai)](https://lmgarret.github.io/polar-flow-mcp/guides/expose-securely/)**
for the complete walkthrough — including why forward-auth goes on `/authorize`
only and which hardening env vars to set.

## Multi-user

This server is single-user by design: it acts as exactly one Polar account.
To serve more than one account, run multiple instances (e.g. one container per
user). The previous OAuth / proxy-auth machinery was removed because the Polar
Flow web API requires email/password and per-user credential storage is a
materially heavier risk profile than per-user OAuth tokens.

## How it works

`internal/flow/` wraps an [ogen](https://github.com/ogen-go/ogen)-generated
Go client against a vendored copy of the OpenAPI spec from the sibling
[polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker) repo.
The wrapper:

- Performs Polar's internal OAuth login chain headlessly (see `internal/flow/login.go`).
- Persists `FLOW_SESSION` + `remember-me` to a chmod-600 cookie jar.
- Injects `X-Requested-With: XMLHttpRequest` on every `/api/*` mutation
  (Play's CSRF filter requires it).
- Detects `401 {"error":"NotAuthenticated"}` and runs a 3-hop silent refresh
  before retrying the request once.

## Disclaimer

This project is not affiliated with, endorsed by, or sponsored by Polar Electro Oy. "Polar" and "Polar Flow" are trademarks of their respective owner.

This software drives an unofficial, reverse-engineered web API that is not publicly documented, may change or break without notice, and whose use may be contrary to Polar's Terms of Service. **Use at your own risk.** The author accepts no responsibility for account suspension or any other consequences arising from its use. Use a dedicated test account where possible.

## License

MIT — see [LICENSE](LICENSE).
