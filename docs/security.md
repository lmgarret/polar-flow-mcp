# Security

This page documents the security model of polar-flow-mcp: the trust boundaries it
enforces, the cryptography it uses, why it is designed to fail closed, and the key
rotation plan for v2.

If you believe you have found a security vulnerability, see the
[Reporting vulnerabilities](#reporting-vulnerabilities) section below.

![Security model trust boundaries](images/security-model.png)

---

## Threat model

### Trust boundaries

polar-flow-mcp sits behind a trusted reverse proxy. There are three trust boundaries:

```
[ User's browser / Claude client ]
            │
            │ HTTPS (TLS terminated by proxy)
            ▼
[ Reverse proxy (Authelia / Authentik / oauth2-proxy / etc.) ]
            │
            │ HTTP + Remote-User header + X-Proxy-Secret header
            ▼
[ polar-flow-mcp server ]
            │
            │ HTTPS (TLS, outbound)
            ▼
[ Polar AccessLink API (flow.polar.com) ]
```

**Boundary 1: User → Reverse proxy.** The reverse proxy is responsible for all
end-user authentication — login forms, MFA, session management, and TLS. polar-flow-mcp
has no role at this boundary. The proxy is assumed to be correctly configured and
trustworthy.

**Boundary 2: Reverse proxy → polar-flow-mcp.** The server enforces this boundary using
a shared secret (`PROXY_SHARED_SECRET`) sent as `X-Proxy-Secret` on every request and
verified using `subtle.ConstantTimeCompare`. A user identity header (`Remote-User` by
default) carries the authenticated identity. The server trusts this identity header
**only after** the shared secret is verified.

**Boundary 3: polar-flow-mcp → Polar AccessLink API.** The server uses a per-user
OAuth2 access token to call the Polar API on behalf of the user. Tokens are encrypted
at rest and decrypted only at call time.

### Who the adversaries are

| Adversary | Threat | Mitigated by |
|-----------|--------|-------------|
| Network attacker (LAN or internet) | Bypass proxy by sending requests directly to polar-flow-mcp | `subtle.ConstantTimeCompare` on every request; `BIND_ADDRESS` defaults to 127.0.0.1 |
| Malicious local user with proxy bypass attempt | Forge identity by sending `Remote-User` header without the shared secret | Shared secret check runs before identity header is read |
| Timing oracle on shared secret | Binary search the secret via response time difference | `subtle.ConstantTimeCompare` (constant time regardless of match) |
| Stolen database file | Read Polar OAuth tokens from copied SQLite file | AES-256-GCM encryption at rest; ciphertext is useless without the key |
| Compromised Polar API | Malicious API responses affecting server | Polar API is out of scope; server does not execute Polar responses as code |

---

## Encryption at rest

### What is encrypted

Every user's Polar OAuth access token is encrypted before being written to SQLite and
decrypted only when an API call needs to be made. No plaintext token is ever written to
disk.

### Algorithm and storage layout

Tokens are encrypted with **AES-256-GCM** using a 12-byte random nonce. The encrypted
blob is stored in the `polar_tokens.encrypted_token` column as a single BLOB with the
following layout:

```
| nonce (12 bytes) | AES-256-GCM ciphertext (variable) |
```

The nonce is prepended to the ciphertext and stored as a single column. This avoids
the nonce/ciphertext misalignment bug that would occur if they were stored in separate
columns and accidentally reassociated.

AES-256-GCM provides both **confidentiality** (encryption) and **integrity** (the GCM
authentication tag detects any modification of the ciphertext). A modified ciphertext
will fail to decrypt and return an error, not corrupted plaintext.

### What this protects

- **Stolen database file:** A copy of `polar.db` without the `ENCRYPTION_KEY` contains
  only ciphertext. An attacker cannot decrypt the tokens without the key.
- **Database backup exposure:** Backups of the volume are safe to store as long as the
  encryption key is stored separately.

### What this does NOT protect

- **Memory dump while server is running:** When a token is actively used to call the
  Polar API, the plaintext exists in server memory. A process memory dump from a
  compromised host would expose the plaintext token.
- **Compromised encryption key:** If the `ENCRYPTION_KEY` environment variable or key
  file is exposed to an attacker who also has a database copy, all stored tokens are
  compromised. Store the key with the same care as a password — use Docker secrets,
  Kubernetes secrets, or a secrets manager, not a plain `.env` file in the repo.
- **Active server access:** An attacker with HTTP access to the `/mcp` endpoint and a
  valid identity + proxy secret can call Polar on behalf of any user, regardless of
  encryption. The encryption protects at-rest storage, not active sessions.

---

## Fail-closed rationale

polar-flow-mcp is designed to refuse to start in any state that could expose users or
their data. On startup, the server validates all of the following and exits with a
non-zero code if any check fails:

| Check | Condition | Error if failed |
|-------|-----------|-----------------|
| AUTH_PROXY configured | `AUTH_PROXY ≠ "unconfigured"` and non-empty | Server exits 1 |
| PROXY_SHARED_SECRET set | Non-empty string | Server exits 1 |
| Encryption key loaded | Decodes to exactly 32 bytes | Server exits 1 |
| Database reachable | Both read and write pools ping successfully | Server exits 1 |

The `docker-compose.yml` in the repository ships with:

```yaml
AUTH_PROXY: "unconfigured"
PROXY_SHARED_SECRET: ""
ENCRYPTION_KEY: ""
```

This is a deliberate UX contract for operators. A copy-paste deployment that skips
configuration will fail to start immediately and loudly, rather than silently running
with no authentication. The alternative — defaulting to a permissive mode — is the most
common self-host footgun: operators deploy, think it's working, and only discover the
misconfiguration after a security incident.

The `/readyz` endpoint provides machine-readable check results so you can diagnose
exactly which configuration is missing.

---

## Header contract

### The two required headers

Every request to polar-flow-mcp (except `/healthz` and `/readyz`) must carry:

1. **`X-Proxy-Secret`** — the value of `PROXY_SHARED_SECRET`, injected by the reverse
   proxy.
2. **Identity header** (default `Remote-User`, configurable via `IDENTITY_HEADER`) —
   the authenticated user's identity string, injected by the reverse proxy.

### Verification ordering (non-negotiable)

The shared secret check runs **before** the identity header is read. This ordering is
enforced in `auth.Middleware` and must never be inverted.

The reason: if the identity header were read first and the secret checked second, a
response-time difference between "wrong secret but valid identity" and "wrong secret and
missing identity" could leak whether a given identity header value is valid. By checking
the secret first, any request without the correct secret gets an identical rejection
regardless of what identity it claims.

This verification uses `subtle.ConstantTimeCompare` from Go's `crypto/subtle` package,
which compares two byte slices in constant time regardless of how many bytes match.

### What a correct proxy configuration looks like

Your reverse proxy must inject both headers on every request to polar-flow-mcp:

```
X-Proxy-Secret: <value of PROXY_SHARED_SECRET>
Remote-User: <authenticated username>
```

A configuration that injects only `Remote-User` without `X-Proxy-Secret` is insecure —
any client that can reach polar-flow-mcp directly (bypassing the proxy) can forge the
identity header. See [Auth Proxies](deployment/auth-proxies.md) for configuration
examples for each supported proxy.

---

## Key rotation plan

### v1 status: no rotation tooling

In v1, only one encryption key version is supported. The `key_version` column in the
`polar_tokens` table is always set to `1`. There is no CLI or API for rotating the
encryption key while keeping existing tokens valid.

### Rotating the key in v1 (manual procedure)

If you need to change the `ENCRYPTION_KEY` in v1:

1. All users must re-authorize their Polar account via the `/oauth/login` flow.
2. Stop the server.
3. Delete the `polar_tokens` table contents (or the entire database).
4. Update `ENCRYPTION_KEY` to the new value.
5. Restart the server.
6. Each user visits `/oauth/login` to re-link their Polar account.

### v2 rotation plan

The `key_version` column is already present in the schema to support a future rotation
workflow:

1. A migration tool reads all rows with `key_version = 1`, decrypts with the old key,
   re-encrypts with the new key, and writes `key_version = 2`.
2. The server is updated to try `key_version = 2` first, then fall back to version 1
   for any rows not yet migrated.
3. Once all rows are migrated, the old key can be retired.

This design (deferred to v2, tracked as OPS-02) avoids a forced re-authorization when
the key is rotated. Until v2 ships, the manual procedure above is the only option.

---

## Reporting vulnerabilities

If you discover a security vulnerability in polar-flow-mcp, please report it
responsibly. Do not open a public GitHub issue for security vulnerabilities.

See [SECURITY.md](https://github.com/lmgarret/polar-flow-mcp/blob/main/SECURITY.md)
in the repository root for the disclosure policy and contact information.
