# Exposing securely (Caddy + Authelia + Claude.ai)

This guide exposes the server to the public internet so you can use it as a
**Claude.ai custom connector** (web + mobile), protected by OAuth 2.1. The Go
server becomes a provider-agnostic OAuth **Resource Server**: it serves
discovery metadata and validates every Bearer token by **RFC 7662
introspection** against your identity provider.

The server itself is agnostic — it works with any OIDC issuer that exposes an
introspection endpoint, behind any reverse proxy that terminates TLS, preserves
the `Host` header, and streams Server-Sent Events. This guide uses **Authelia**
as the example identity provider and **Caddy** as the example reverse proxy.
(Other providers/proxies such as nginx can be added later — the moving parts are
called out so they're easy to translate.)

!!! info "Which Claude clients need this"
    - **Claude.ai web + mobile** — yes; they reach your server over the public
      internet and require this OAuth setup.
    - **Claude Desktop** — uses the same remote-connector OAuth path.
    - **Claude Code (CLI)** — **no.** Claude Code forces Dynamic Client
      Registration and ignores a static client id
      ([anthropics/claude-code#38102](https://github.com/anthropics/claude-code/issues/38102)),
      which Authelia does not support. Keep Claude Code on the **local stdio /
      `127.0.0.1`** transport it already uses — nothing is lost.

## Why not Authelia forward-auth?

Forward-auth (`/api/authz/forward-auth`) authenticates by **redirecting a
browser to a login portal and setting a session cookie**. Claude's MCP clients
are not browsers following redirects — they speak **OAuth 2.1 Bearer**: they hit
`/mcp`, get a `401` with a `WWW-Authenticate` header, discover your
authorization server, and attach `Authorization: Bearer <token>`. A
forward-auth cookie handshake can never complete for them. So Authelia is used
here as the **OIDC authorization server** (the thing that logs *you* in and
issues tokens), **not** as forward-auth in front of `/mcp`.

Authelia also does not yet support Dynamic Client Registration or Client ID
Metadata Documents. That's fine: **Claude.ai accepts a pre-registered client** —
you register one client in Authelia and paste its id/secret into the connector's
advanced settings.

## Topology

```
Claude.ai web/mobile ──HTTPS──► Caddy (TLS) ──► polar-flow-mcp :8080 /mcp
        │ 401 + WWW-Authenticate → /.well-known/oauth-protected-resource
        ▼
  Authelia (OIDC): you log in (browser) → authorize + token  ─┐
        ▼                                                      │ Bearer token
  POST /mcp  Authorization: Bearer <opaque> ──► server introspects at Authelia ──► tools
```

## Identity provider — example: Authelia

You need **one** OIDC client on Authelia — the connector Claude uses to log you
in. Because it's a confidential client, the MCP server reuses **its** credentials
to validate tokens at Authelia's introspection endpoint, so no second client is
required. (For stricter credential separation you can split introspection into
its own client — see
[Optional: a separate introspection client](#optional-a-separate-introspection-client).)

```yaml
# authelia configuration.yml
identity_providers:
  oidc:
    clients:
      # The client Claude uses (pre-registered; you paste these into Claude.ai).
      # It doubles as the MCP server's introspection credential.
      - client_id: 'polar-mcp-connector'
        client_name: 'Polar Flow MCP (Claude)'
        client_secret: '$pbkdf2-sha512$310000$...'   # hashed; keep the plaintext for Claude + the server
        public: false
        authorization_policy: 'two_factor'            # restrict to YOU (see note)
        require_pkce: true
        pkce_challenge_method: 'S256'                 # Claude always uses PKCE S256
        redirect_uris:
          - 'https://claude.ai/api/mcp/auth_callback'
          - 'https://claude.com/api/mcp/auth_callback'
        scopes: ['openid', 'profile', 'groups', 'offline_access']  # offline_access → refresh tokens
        audience: ['https://polar.example.com/mcp']   # RFC 8707 — stamps the aud claim
        grant_types: ['authorization_code', 'refresh_token']
        response_types: ['code']
        token_endpoint_auth_method: 'client_secret_post'           # how Claude authenticates
        introspection_endpoint_auth_method: 'client_secret_basic'  # how the server authenticates (HTTP Basic)
```

!!! tip "Lock the connector client to you"
    Set the connector's `authorization_policy` (or an ACL by user/group) so only
    **you** may complete its login. Then Authelia won't even *issue* a token to
    anyone else — defence in depth before the server validates anything. This is
    the single most effective control if your Authelia has more than one user.

!!! warning "Opaque tokens are expected"
    Authelia issues **opaque** access tokens by default. That's exactly why this
    server validates by **introspection** rather than local JWT checks — no extra
    Authelia token-format config, and you get **instant revocation** (logging out
    in Authelia kills the token on its next use).

!!! note "Auth-method mismatch?"
    `token_endpoint_auth_method: 'client_secret_post'` matches how Claude posts
    its credentials. If the login fails with a client-authentication error, try
    `'client_secret_basic'`. Whichever client the server introspects with must
    set `introspection_endpoint_auth_method: 'client_secret_basic'` — that's the
    scheme this server sends.

### Optional: a separate introspection client

Reusing the connector client means one secret does two jobs — Claude's login and
the server's introspection. The tradeoff is **credential rotation**: that secret
also lives in the server's environment, and rotating it (say it leaks from the
host) forces you to re-paste the new value into Claude.ai's connector settings,
because it's the same secret. Token revocation is unaffected — only secret
rotation loses its independence.

If you'd rather rotate the server's credential without ever touching the Claude
connector, register a second confidential client and point the server's
`OIDC_INTROSPECTION_*` vars at it instead:

```yaml
      # Optional 2nd client: dedicated introspection credentials for the server.
      - client_id: 'polar-mcp-rs'
        client_name: 'Polar Flow MCP (introspection)'
        client_secret: '$pbkdf2-sha512$310000$...'
        public: false
        authorization_policy: 'one_factor'
        scopes: ['openid']
        grant_types: ['client_credentials']
        introspection_endpoint_auth_method: 'client_secret_basic'
```

Then set `OIDC_INTROSPECTION_CLIENT_ID=polar-mcp-rs` (and its secret) below.
`AUTH_ALLOWED_CLIENT_IDS` still pins the **connector** (`polar-mcp-connector`) —
that's the client named in the *token*, not the one doing the introspecting.

Make sure Authelia's discovery + endpoints are publicly reachable over HTTPS:
`https://auth.example.com/.well-known/openid-configuration` and the
`authorization`, `token`, and `introspection` endpoints it lists.

## Reverse proxy — example: Caddy

Caddy provides the public TLS certificate and forwards to the container. A
single `reverse_proxy` is enough. **Do not** add a forward-auth directive on
`/mcp` — auth is handled by the server via OAuth.

```caddyfile
polar.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

The two paths that matter are served by polar-flow-mcp directly through this:

- `https://polar.example.com/.well-known/oauth-protected-resource`
- `https://polar.example.com/mcp`

**Why this works out of the box with Caddy** — and what any other proxy (nginx,
Traefik, …) must replicate:

- **Host preserved.** Caddy passes the incoming `Host` through unchanged to an
  HTTP upstream, so `MCP_RESOURCE` matches the URL Claude calls. On other
  proxies, preserve `Host` explicitly (nginx: `proxy_set_header Host $host;`).
- **SSE streamed, not buffered.** The `/mcp` transport returns
  `Content-Type: text/event-stream`; Caddy auto-flushes these immediately with no
  config. Other proxies must disable response buffering for that path (nginx:
  `proxy_buffering off;`).
- **No proxy read/write timeout needed.** Caddy's reverse-proxy transport has no
  default stream timeout, so long tool responses aren't cut. On proxies with
  default timeouts, raise/disable them for `/mcp`.
- **Path pass-through.** `/mcp` and `/.well-known/*` must reach the server
  unrewritten.

!!! danger "Exact-match the resource URL"
    The most common failure is a mismatch between `MCP_RESOURCE`, the URL you
    enter in Claude, and what the proxy forwards (scheme, host, **trailing
    slash**). Keep all three identical, e.g. `https://polar.example.com/mcp`.

## polar-flow-mcp — enable the Resource Server

Add to the server's environment (see also
[Environment Variables](../reference/env-vars.md)). Setting `OIDC_ISSUER` turns
auth on; the server refuses to start if the resource or introspection
credentials are missing.

```bash
OIDC_ISSUER=https://auth.example.com
MCP_RESOURCE=https://polar.example.com/mcp        # MUST match the URL Claude calls, byte for byte
OIDC_INTROSPECTION_CLIENT_ID=polar-mcp-connector  # reuse the connector client (or polar-mcp-rs if you split it)
OIDC_INTROSPECTION_CLIENT_SECRET=<plaintext secret for that client>

# Recommended hardening — each enforced only when set (all ANDed):
AUTH_ALLOWED_CLIENT_IDS=polar-mcp-connector       # confused-deputy defence (RFC 8707)
AUTH_ALLOWED_AUDIENCES=https://polar.example.com/mcp
AUTH_ALLOWED_SUBJECTS=<your-authelia-sub-uuid>    # see note below
# AUTH_ALLOWED_GROUPS=polar                         # alternative to subject for multi-user
# AUTH_ALLOWED_ORIGINS=https://claude.ai            # opt-in DNS-rebinding defence
```

!!! note "Finding your subject"
    Authelia's `sub` is a random **UUID**, not your username. Connect once with
    `AUTH_ALLOWED_SUBJECTS` unset, then read the server log line on a request —
    it prints the observed `sub`/`client_id`/`aud` so you can copy the right
    value. If your Authelia is single-user, you can skip the subject pin and rely
    on the client/audience pins + the Authelia authorization policy.

## Add the connector in Claude.ai

1. **Settings → Connectors → Add custom connector**.
2. **URL**: `https://polar.example.com/mcp`.
3. **Advanced settings** → paste the **`polar-mcp-connector`** client id and
   client secret.
4. Save and connect. Claude opens a browser to Authelia; log in; approve.
5. The tools (`get_user_info`, …) should now list.

## Verify

```bash
# Discovery metadata is public:
curl -s https://polar.example.com/.well-known/oauth-protected-resource | jq

# Unauthenticated /mcp returns 401 pointing at the metadata:
curl -i -X POST https://polar.example.com/mcp
# → HTTP/1.1 401 Unauthorized
# → WWW-Authenticate: Bearer resource_metadata="https://polar.example.com/.well-known/oauth-protected-resource"
```

Then end-to-end: connect in Claude.ai, run `get_user_info`, and confirm it
succeeds. **Revoke** the session in Authelia (log out everywhere) and confirm the
next call fails — that proves introspection-based revocation is working.

## Security checklist

- [ ] Proxy serves a valid TLS cert; HTTP is redirected to HTTPS.
- [ ] No forward-auth directive on `/mcp` (it would break Claude).
- [ ] `MCP_RESOURCE` == the connector URL == what the proxy forwards (incl. slash).
- [ ] `AUTH_ALLOWED_CLIENT_IDS` set to the connector client (confused-deputy defence).
- [ ] `AUTH_ALLOWED_SUBJECTS` (or `AUTH_ALLOWED_GROUPS`) restricts to you, **or**
      the connector client is locked to you by an Authelia authorization policy.
- [ ] Short access-token lifetime + refresh tokens in Authelia.
- [ ] A **dedicated** Polar account (not your main one) — see [Security](../security.md).
- [ ] Container bound to localhost/private, reachable only through the proxy.

## Using a different provider or proxy

The server is provider- and proxy-agnostic. Any OIDC issuer with an
introspection endpoint works in place of Authelia (point `OIDC_ISSUER` at it);
any reverse proxy works in place of Caddy as long as it terminates TLS,
preserves `Host`, streams `text/event-stream` without buffering, and passes
`/mcp` + `/.well-known/*` through unrewritten. Worked examples for other stacks
(e.g. nginx) may be added here later.
