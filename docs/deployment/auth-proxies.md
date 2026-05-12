# Auth Proxy Configuration

polar-flow-mcp requires a trusted reverse proxy to handle user authentication. This page
provides configuration snippets for five commonly used proxies. Any proxy that can inject
the required headers will work — the five proxies below are tested and documented.

![Authelia login screen](../images/authelia-login.png)

---

## Header contract

Regardless of which proxy you use, it must inject two headers on **every** request
forwarded to polar-flow-mcp:

| Header | Value |
|--------|-------|
| `Remote-User` (or your `IDENTITY_HEADER`) | Authenticated user's identity string |
| `X-Proxy-Secret` | The value of `PROXY_SHARED_SECRET` |

The server verifies `X-Proxy-Secret` first using `subtle.ConstantTimeCompare`, then reads
the identity header. A request that has a valid identity header but the wrong (or absent)
`X-Proxy-Secret` is rejected with `403 Forbidden`.

You can change the identity header name by setting `IDENTITY_HEADER` in your
`docker-compose.yml`. The examples below use `Remote-User` (the default).

---

## Authelia

Authelia is an open-source SSO and access control server commonly used in homelabs. It
sits in front of your services as a reverse proxy companion, authenticating users via a
login portal and injecting identity headers into forwarded requests.

Authelia natively injects the `Remote-User` header on authenticated requests — this
matches polar-flow-mcp's default `IDENTITY_HEADER`.

### Configuration

In your Authelia `configuration.yml`, define a rule that protects the polar-flow-mcp
host and passes requests through after authentication:

```yaml
# authelia/configuration.yml (relevant excerpt)
access_control:
  default_policy: deny
  rules:
    - domain: polar.example.com
      policy: one_factor       # or two_factor for stronger auth
```

In your NGINX or Traefik configuration (the actual reverse proxy that Authelia protects),
add the `X-Proxy-Secret` header injection. With NGINX + Authelia's `auth_request`:

```nginx
# nginx.conf — polar-flow-mcp location block
location / {
    auth_request /authelia;
    auth_request_set $user $upstream_http_remote_user;

    proxy_set_header Remote-User $user;
    proxy_set_header X-Proxy-Secret "your-proxy-shared-secret";
    proxy_pass http://127.0.0.1:8080;
}

location /authelia {
    internal;
    proxy_pass http://authelia:9091/api/authz/auth-request;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URL $scheme://$http_host$request_uri;
}
```

With Traefik + Authelia's forward auth middleware:

```yaml
# traefik dynamic config (e.g., polar.yml)
http:
  routers:
    polar:
      rule: Host(`polar.example.com`)
      middlewares:
        - authelia
        - polar-headers
      service: polar-svc
      tls: {}

  middlewares:
    authelia:
      forwardAuth:
        address: http://authelia:9091/api/authz/forward-auth
        trustForwardHeader: true
        authResponseHeaders:
          - Remote-User
          - Remote-Groups
          - Remote-Name
          - Remote-Email
    polar-headers:
      headers:
        customRequestHeaders:
          X-Proxy-Secret: "your-proxy-shared-secret"

  services:
    polar-svc:
      loadBalancer:
        servers:
          - url: http://polar-flow-mcp:8080
```

Set `AUTH_PROXY: "authelia"` in your `docker-compose.yml`.

---

## Authentik

Authentik is a self-hosted identity provider that supports SSO, SAML, OAuth2, and
LDAP. It provides a proxy provider that injects identity headers into forwarded requests.

Authentik's Proxy Provider supports both forward auth (for NGINX/Traefik) and embedded
outpost mode. In both cases, you configure which headers are passed downstream.

### Configuration

Create a Proxy Provider in Authentik for polar-flow-mcp:

1. In Authentik admin, go to **Applications → Providers → Create → Proxy Provider**.
2. Set **External host** to `https://polar.example.com`.
3. Set **Internal host** to `http://polar-flow-mcp:8080`.
4. Under **Advanced protocol settings**, add a custom header:

```yaml
# Authentik Proxy Provider — advanced settings (UI configuration)
# Add this under "Additional headers to be passed to the upstream"
X-Proxy-Secret: "your-proxy-shared-secret"
```

Authentik injects `X-authentik-username` by default. To use `Remote-User` instead,
configure header mapping in the Proxy Provider:

```yaml
# Authentik property mappings (scope-level configuration)
# Map the authenticated username to Remote-User
Remote-User: "{{ request.user.username }}"
```

Alternatively, set `IDENTITY_HEADER: "X-authentik-username"` in your
`docker-compose.yml` to use Authentik's native header.

With Traefik outpost mode, the Traefik configuration is managed by Authentik
automatically. You only need to ensure the custom `X-Proxy-Secret` header is injected.

Set `AUTH_PROXY: "authentik"` in your `docker-compose.yml`.

---

## oauth2-proxy

oauth2-proxy is a lightweight reverse proxy that provides OAuth2 and OIDC
authentication in front of your applications. It is commonly used with GitHub, Google,
and any OIDC provider (Keycloak, Dex, etc.).

oauth2-proxy injects `X-Auth-Request-User` by default (not `Remote-User`). Set
`IDENTITY_HEADER: "X-Auth-Request-User"` in your `docker-compose.yml`, or configure
oauth2-proxy to use `Remote-User` as shown below.

### Configuration

```ini
# oauth2-proxy.cfg

# Upstream: polar-flow-mcp
upstreams = ["http://polar-flow-mcp:8080/"]

# Authentication provider (example: GitHub)
provider = "github"
client_id = "<github-oauth-app-client-id>"
client_secret = "<github-oauth-app-client-secret>"

# Cookie/session settings
cookie_secret = "<random-32-byte-base64>"
cookie_secure = true

# Header injection
# Set the header name to match IDENTITY_HEADER in polar-flow-mcp
# Default is X-Auth-Request-User; use Remote-User to match polar-flow-mcp default
set_xauthrequest = true
pass_user_headers = true

# Inject X-Proxy-Secret via response headers
# Note: oauth2-proxy does not natively support injecting custom request headers.
# Use your frontend proxy (NGINX/Traefik) to add X-Proxy-Secret after oauth2-proxy
# authenticates the request.
```

Since oauth2-proxy does not support injecting arbitrary request headers, add the
`X-Proxy-Secret` header at the frontend proxy layer. With NGINX:

```nginx
# NGINX — after oauth2-proxy auth_request
location / {
    auth_request /oauth2/auth;
    auth_request_set $user $upstream_http_x_auth_request_user;

    proxy_set_header X-Auth-Request-User $user;
    proxy_set_header X-Proxy-Secret "your-proxy-shared-secret";
    proxy_pass http://polar-flow-mcp:8080;
}

location /oauth2/ {
    proxy_pass http://oauth2-proxy:4180;
    proxy_set_header Host $host;
}
```

Set `AUTH_PROXY: "oauth2-proxy"` and `IDENTITY_HEADER: "X-Auth-Request-User"` in your
`docker-compose.yml`.

---

## Pomerium

Pomerium is an identity-aware proxy that integrates with OIDC providers (Okta, Auth0,
Google Workspace, etc.) and supports mutual TLS and device identity. It is well-suited
for zero-trust architectures.

Pomerium injects signed JWT claims as headers. You can configure which claims map to
which headers, making it easy to inject both the user identity and the proxy secret.

### Configuration

```yaml
# pomerium config.yaml (relevant policy excerpt)

policy:
  - from: https://polar.example.com
    to: http://polar-flow-mcp:8080
    allow_any_authenticated_user: true

    # Inject the authenticated user's email as Remote-User
    set_request_headers:
      Remote-User: "{{ .email }}"
      X-Proxy-Secret: "your-proxy-shared-secret"
```

With Pomerium's `set_request_headers`, both headers are injected for every authenticated
request that matches the policy. The `{{ .email }}` template populates the user's
authenticated email address as the identity value.

If you prefer to use the user's username or subject claim instead of email:

```yaml
    set_request_headers:
      Remote-User: "{{ .user }}"
      X-Proxy-Secret: "your-proxy-shared-secret"
```

Pomerium also supports per-route header injection in its Kubernetes ingress
annotations if you are running in a Kubernetes environment.

Set `AUTH_PROXY: "pomerium"` in your `docker-compose.yml`.

---

## Cloudflare Access

Cloudflare Access is a zero-trust network access product that sits in front of your
self-hosted applications, authenticating users via identity providers (Cloudflare,
Google, GitHub, SAML/OIDC). Requests are proxied through Cloudflare's edge before
reaching your server.

Cloudflare Access injects a signed JWT (`Cf-Access-Jwt-Assertion` header) and the
authenticated user's email (`Cf-Access-Authenticated-User-Email` header) into every
forwarded request.

### Configuration

In your Cloudflare Zero Trust dashboard:

1. Go to **Access → Applications → Add an application**.
2. Choose **Self-hosted**.
3. Set the application domain to `polar.example.com`.
4. Configure an access policy (e.g., allow users in your organization).

On your origin server (NGINX or Caddy behind Cloudflare), inject `X-Proxy-Secret` and
map Cloudflare's identity header to `Remote-User`:

```nginx
# NGINX origin server (behind Cloudflare Access)
server {
    listen 8443;

    location / {
        # Map Cloudflare's identity header to Remote-User
        proxy_set_header Remote-User $http_cf_access_authenticated_user_email;
        proxy_set_header X-Proxy-Secret "your-proxy-shared-secret";
        proxy_pass http://polar-flow-mcp:8080;
    }
}
```

With Caddy:

```caddy
polar.example.com {
    reverse_proxy polar-flow-mcp:8080 {
        header_up Remote-User {header.Cf-Access-Authenticated-User-Email}
        header_up X-Proxy-Secret "your-proxy-shared-secret"
    }
}
```

> **Important:** Cloudflare Access delivers the authenticated user's email address, not
> a username. Your polar-flow-mcp user identities will be email addresses (e.g.,
> `alice@example.com`). This is fine — the identity is just a string used as a database
> key.

Set `AUTH_PROXY: "cloudflare-access"` and optionally
`IDENTITY_HEADER: "Cf-Access-Authenticated-User-Email"` in your `docker-compose.yml`
(or leave `IDENTITY_HEADER` as the default `Remote-User` if your origin server maps it
as shown above).

---

## Any other proxy

The header contract is the only requirement: inject `Remote-User` (or your chosen
`IDENTITY_HEADER`) and `X-Proxy-Secret` on every request. Any proxy or load balancer
that can inject custom headers will work.

Common alternatives that work with the same pattern:

- **Nginx Proxy Manager** — add custom headers in the Advanced tab of your proxy host
- **Caddy** — use `header_up` directives in the reverse proxy block
- **HAProxy** — use `http-request set-header` in the backend configuration
- **Vouch Proxy** — similar to oauth2-proxy; inject `X-Proxy-Secret` at the NGINX layer
