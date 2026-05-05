# Phase 2: OAuth Link Flow + UserInfo - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-05
**Phase:** 2-OAuth Link Flow + UserInfo
**Areas discussed:** Redirect URI strategy, Polar credentials at startup, Callback response format, get_user_info data source

---

## Redirect URI Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| POLAR_REDIRECT_URL env var | Operator sets once at deploy time; matches what they registered with Polar; required at startup | ✓ |
| Derived from request headers | Built from r.Host + X-Forwarded-Proto; zero config but scheme detection can misfire behind a proxy | |

**User's choice:** POLAR_REDIRECT_URL env var
**Notes:** User asked for clarification on what redirect URI is used for. Explained: it's a required OAuth2 parameter sent to Polar so Polar knows where to redirect the user after authorization, and it must exactly match what was registered in the Polar developer portal. Since operators must register the URL with Polar at setup time anyway, an env var is a natural fit — set once, never touch again.

---

## Polar Credentials at Startup

| Option | Description | Selected |
|--------|-------------|----------|
| Fail at startup if missing | Consistent with Phase 1 fail-closed pattern; server refuses to start with actionable error | ✓ |
| Fail on first OAuth attempt | Lighter startup; but silent misconfiguration until a user actually tries to link | |

**User's choice:** Fail at startup
**Notes:** Consistent with established Phase 1 pattern (AUTH_PROXY, PROXY_SHARED_SECRET, ENCRYPTION_KEY all required at startup). POLAR_CLIENT_ID, POLAR_CLIENT_SECRET, and POLAR_REDIRECT_URL added to Config struct and validated in Load().

---

## Callback Response Format

| Option | Description | Selected |
|--------|-------------|----------|
| Plain text | Simple, no deps; success 200 + text; errors 400/500 + message | |
| Inline HTML | Self-contained HTML page, no external deps, inline CSS, slightly nicer | ✓ |

**User's choice:** Inline HTML
**Notes:** Same HTML shell for success and error pages. No external dependencies, no JS. Success: checkmark + "Polar account linked" + "You may close this tab and return to Claude." Errors: appropriate HTTP status + clear message.

---

## get_user_info Data Source

| Option | Description | Selected |
|--------|-------------|----------|
| Local DB only | Returns polar_user_id + identity from store; fast, no Polar API call, no failure mode | ✓ |
| Live Polar API call | Calls GET /v3/users/{id} for name/email; richer but adds latency and a failure mode | |

**User's choice:** Local DB only
**Notes:** Returns linked bool, polar_user_id (int64), and identity string. If not linked, includes hint to visit /oauth/login.

---

## Claude's Discretion

- Error message copy for HTML error pages (wording is an implementation detail)
- HTML/CSS styling for callback pages (minimal, self-contained)
- Whether to use `golang.org/x/oauth2` convenience types or raw `net/http` for token exchange

## Deferred Ideas

None — discussion stayed within phase scope.
