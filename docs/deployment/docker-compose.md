# Docker Compose Deployment

This guide walks through every section of the `docker-compose.yml` file included in the
repository, explaining what each setting does and how to fill in the placeholders.

---

## Overview

The `docker-compose.yml` defines a single service (`polar-flow-mcp`) that runs the
server with a persistent SQLite database stored in a named volume.

![docker compose up output](../images/docker-compose-up.png)

---

## The service definition

```yaml
services:
  polar-flow-mcp:
    image: ghcr.io/lmgarret/polar-flow-mcp:latest
    restart: unless-stopped
    volumes:
      - polar-data:/data
    environment:
      AUTH_PROXY: "unconfigured"
      PROXY_SHARED_SECRET: ""
      ENCRYPTION_KEY: ""
      POLAR_CLIENT_ID: ""
      POLAR_CLIENT_SECRET: ""
      DATABASE_PATH: "/data/polar.db"
```

### `image`

```yaml
image: ghcr.io/lmgarret/polar-flow-mcp:latest
```

The published Docker image from GitHub Container Registry. Always use
`ghcr.io/lmgarret/polar-flow-mcp` — this is the correct public image path.

> **Note:** The `docker-compose.yml` file committed to the repository contains
> `ghcr.io/lm/polar-flow-mcp:latest` — this is the old Go module-derived path and will
> not be updated until Phase 4 release. Replace it with
> `ghcr.io/lmgarret/polar-flow-mcp:latest` in your local copy.

To pin to a specific release instead of `latest`:

```yaml
image: ghcr.io/lmgarret/polar-flow-mcp:1.0.0
```

### `restart: unless-stopped`

Restarts the container automatically if it exits (e.g., after a server reboot), unless
you explicitly stop it with `docker compose stop`. This is appropriate for homelab
deployments that should survive reboots.

### `volumes`

```yaml
volumes:
  - polar-data:/data
```

Mounts the named volume `polar-data` at `/data` inside the container. The SQLite
database is stored at `/data/polar.db` (set via `DATABASE_PATH`). Without this volume,
the database is lost when the container is removed.

The named volume `polar-data` is declared at the bottom of the file:

```yaml
volumes:
  polar-data:
```

Docker manages the volume lifecycle. To back up the database:

```bash
docker run --rm -v polar-flow-mcp_polar-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/polar-db-backup.tar.gz /data/polar.db
```

---

## Environment variables

### Required variables

These variables must be set before the server will start. The server exits with a
non-zero code if any are missing or at their placeholder defaults.

#### `AUTH_PROXY`

```yaml
AUTH_PROXY: "unconfigured"
```

**Replace with** the name of your reverse proxy: `authelia`, `authentik`,
`oauth2-proxy`, `pomerium`, or `cloudflare-access`. This value is informational — it
appears in the startup log and helps you confirm which proxy is configured.

The server refuses to start when this value is `unconfigured`. This is intentional: a
copy-paste deployment that skips configuration fails loudly rather than silently running
without authentication.

#### `PROXY_SHARED_SECRET`

```yaml
PROXY_SHARED_SECRET: ""
```

**Replace with** a random hex string. Generate one with:

```bash
openssl rand -hex 32
```

This secret is shared between your reverse proxy and polar-flow-mcp. Your proxy must
inject it as the `X-Proxy-Secret` header on every request. The server verifies it using
`subtle.ConstantTimeCompare` before reading any other headers.

Keep this value confidential. If it leaks, an attacker who can reach
polar-flow-mcp directly could bypass the proxy authentication.

#### `ENCRYPTION_KEY`

```yaml
ENCRYPTION_KEY: ""
```

**Replace with** a 32-byte AES-256-GCM key, base64-encoded. Generate one with:

```bash
openssl rand -base64 32
```

This key encrypts Polar OAuth tokens stored in SQLite. It must remain consistent across
server restarts — if you change it after tokens are stored, all users must re-link their
Polar accounts.

Store this key outside the `docker-compose.yml` file if possible. Use a Docker secret
or an external secrets manager for production deployments.

#### `POLAR_CLIENT_ID` and `POLAR_CLIENT_SECRET`

```yaml
POLAR_CLIENT_ID: ""
POLAR_CLIENT_SECRET: ""
```

**Replace with** the credentials from your Polar developer application. See
[Polar OAuth Setup](polar-oauth-setup.md) for registration instructions.

### Optional variables

These variables have defaults that work for most deployments.

#### `DATABASE_PATH`

```yaml
DATABASE_PATH: "/data/polar.db"
```

The path to the SQLite database inside the container. This is set to `/data/polar.db`
to place the database inside the named volume. Do not change this unless you have a
specific reason.

#### `IDENTITY_HEADER` (commented out)

```yaml
# IDENTITY_HEADER: "Remote-User"
```

The HTTP header your reverse proxy injects for the user identity. Defaults to
`Remote-User`. Uncomment and change if your proxy uses a different header (e.g.,
`X-Auth-Request-User` for oauth2-proxy).

#### `BIND_ADDRESS` (commented out)

```yaml
# BIND_ADDRESS: "127.0.0.1"
```

The IP address the server listens on. Defaults to `127.0.0.1` (loopback only). If
your reverse proxy runs in a separate Docker network, set this to `0.0.0.0` to accept
connections from the Docker bridge network.

---

## Filled example

Here is a complete `docker-compose.yml` with all values filled in (with
`<replace-me>` placeholders for your actual secrets):

```yaml
services:
  polar-flow-mcp:
    image: ghcr.io/lmgarret/polar-flow-mcp:latest
    restart: unless-stopped
    volumes:
      - polar-data:/data
    environment:
      AUTH_PROXY: "authelia"
      PROXY_SHARED_SECRET: "<output of: openssl rand -hex 32>"
      ENCRYPTION_KEY: "<output of: openssl rand -base64 32>"
      POLAR_CLIENT_ID: "<from admin.polaraccesslink.com>"
      POLAR_CLIENT_SECRET: "<from admin.polaraccesslink.com>"
      DATABASE_PATH: "/data/polar.db"
      # IDENTITY_HEADER: "Remote-User"  # uncomment to override
      # BIND_ADDRESS: "0.0.0.0"         # uncomment for Docker network

volumes:
  polar-data:
```

---

## Starting and stopping

```bash
# Start in the background
docker compose up -d

# View logs
docker compose logs -f polar-flow-mcp

# Stop (preserves the volume)
docker compose stop

# Remove the container (preserves the volume)
docker compose down

# Remove the container AND the database volume (DESTRUCTIVE)
docker compose down -v
```

---

## Upgrading

To pull a new image and restart:

```bash
docker compose pull
docker compose up -d
```

Database migrations run automatically on startup. Downgrading is not supported if a
forward migration has run.

---

## Verifying the deployment

After `docker compose up -d`:

```bash
# Check liveness
curl https://<your-host>/healthz
# → ok

# Check readiness (all four checks should be "ok")
curl https://<your-host>/readyz
```

If `/readyz` shows any failing check, the error message tells you exactly what to fix.
See [Reference: Environment Variables](../reference/env-vars.md) for variable details.
