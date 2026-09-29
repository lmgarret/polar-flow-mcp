# internal/flow — Polar Flow web API client

Reverse-engineered client for `flow.polar.com`'s internal web API, generated
from the OpenAPI spec in [`../../../polar-openapi-maker/`](https://github.com/lm/polar-openapi-maker)
(sibling repo) using [ogen](https://github.com/ogen-go/ogen).

> **Not the official AccessLink API.** Auth is cookie-based (`FLOW_SESSION`
> JWT) obtained by running Polar's internal OAuth login chain headlessly with
> a user-provided email + password. See `polar-openapi-maker/docs/auth.md`.

## Layout

| Path | Purpose |
|------|---------|
| `openapi.yaml` | Vendored, preprocessed copy of the upstream spec (ogen input) |
| `preprocess-spec.py` | Converts OpenAPI 3.1 → 3.0.3 (ogen v1 doesn't support 3.1 nullable union syntax) |
| `gen/` | Generated ogen code (committed; regenerate via the `Regeneration` flow below) |
| `client.go`, `login.go`, `cookies.go`, `jwt.go`, `errors.go`, `ops.go`, `transport_tls.go` | Hand-written wrapper around `gen.Client` |
| `ops_favorites.go` | Favorites, routes and scheduling (`/api/favoritetarget`, `/api/favorites/*`, `/training/target/createTargetFromFavourite`) |
| `ops_sessions.go` | Session existence check, delete, full edit (`editTraining`) and partial note/feeling update |
| `sports.go` | Cached sport catalogue behind `SportName` (sport-id validation for write tools) |
| `testing.go` | `NewForTesting(baseURL)` — a Client against an `httptest` server, no login (tests only) |

## Regeneration

```bash
# 1. Re-preprocess the upstream spec
python3 internal/flow/preprocess-spec.py \
    ../polar-openapi-maker/dist/openapi.yaml \
    internal/flow/openapi.yaml

# 2. Re-run ogen
go install github.com/ogen-go/ogen/cmd/ogen@latest
ogen --target internal/flow/gen --package gen --clean internal/flow/openapi.yaml
```

The preprocessor is intentionally tiny — it converts the OpenAPI 3.1
`type: [X, "null"]` union syntax to the equivalent 3.0 `type: X, nullable: true`
form and numeric `exclusiveMinimum`/`exclusiveMaximum` to the 3.0 boolean form,
because ogen v1.x only accepts 3.0, and renames case-colliding properties. No
other semantic changes.

## Wire quirks the wrapper absorbs

- **JSON labelled `text/plain`.** `saveName`, `saveSport`, the favorite delete,
  `favoriteTargetsJson` and `createTargetFromFavourite` send JSON bodies with
  `Content-Type: text/plain`. The spec declares `text/plain` so ogen accepts
  them; `CreateTargetFromFavorite` decodes the body with the generated
  `TargetFromFavorite` model.
- **Silent successes.** `DELETE /api/training/deleteTrainingSession/{id}/`
  answers 200 for ids that do not exist, and `saveSport` answers 200 for
  unknown sport or exercise-target ids. Callers check existence first
  (`CheckTrainingSession`, `GetFavorite`) and read back after.
- **Opaque 500s.** `editTraining` answers every out-of-range field with an empty
  500; `EditTrainingSession` turns it into an explanatory error, and the MCP
  tool validates the ranges before sending.
- **Opaque 400s and a time clash.** Over-long target/phase names answer the
  same generic message as the name content filter, and a create rejected for a
  phase name still stores a half-created, calendar-invisible target that holds
  its time slot — so the MCP tools validate name (≤ 45), description (≤ 500)
  and phase names (≤ 45, UTF-16 code units, `convert.PolarTextLen`) first. A
  create/update onto an occupied minute is a text/plain 400 mapped to
  `ErrTargetTimeClash`.
- **Non-idempotent target delete.** `DELETE /training/target/{id}` answers 400
  text/plain for a missing (`deleteError` → `ErrTargetNotFound`) or foreign
  (→ `ErrNotOwned`) id; `delete_training_target` reads the target first.
- **Opaque fields.** `supportedDeviceIds` in `/api/favorites` is not an int
  array on device-paired accounts; it is left untyped (`jx.Raw`).
- **403 instead of 404.** The route-geometry endpoint answers 403 for any id
  that is not one of the account's exercise targets; it maps to
  `ErrFavoriteNotFound`. Reads of another account's session / favorite map to
  `ErrNotOwned`.

## Architecture

```
    +-----------------+
    |  MCP handlers   |
    +-----------------+
            │
            ▼
    +-----------------+     SessionCookie via SecuritySource
    |  flow.Client    |◄──────────────────────────────────┐
    +-----------------+                                    │
            │                                              │
            ▼                                              │
    +-----------------+     custom http.Client:            │
    |  gen.Client     |────►   - inject X-Requested-With   │
    +-----------------+        - on 401: silent refresh────┘
                               - update cookie jar
```

- `gen.Client` does the HTTP grunt work (URL building, JSON, validation).
- The custom `http.Client` we pass via `gen.WithClient` is a thin RoundTripper that:
  1. Reads the current `FLOW_SESSION` cookie from the session manager.
  2. Adds `X-Requested-With: XMLHttpRequest` on every non-GET request (Play's
     CSRF filter requires it on all mutations, including non-`/api/*` paths
     like `DELETE /training/target/{id}`).
  3. On the **first** request with no session yet, runs the deferred cold-start
     full login (`EnsureSession`, coalesced under the mutex) and swaps the fresh
     `FLOW_SESSION` onto the request.
  4. Detects `401 {"error":"NotAuthenticated"}` and triggers a 3-hop silent
     refresh (see `auth.md`), then retries the request once.
- The session manager owns the cookie jar file (chmod 600). `New` does **not**
  log in — it returns immediately so the server's listener can bind without
  waiting on the login round-trip. The cold-start full login is deferred to the
  first request (and warmed up in the background by `cmd/polar-flow-mcp`); it
  runs only when no usable jar exists.

## CloudFront WAF bypass

`flow.polar.com` sits behind CloudFront with a managed bot ruleset that
inspects **both** the TLS ClientHello (JA3/JA4) and the HTTP/2 framing
fingerprint. Go's default `net/http` + `crypto/tls` produces a non-browser
signature on both layers; the WAF returns `403 X-Cache: Error from
cloudfront` on the `/flowSso/redirect` OAuth code-exchange hop.

Two pieces address this:

- **Login chain** (`login.go`) runs through a transient
  [`azuretls.Session`](https://github.com/Noooste/azuretls-client) preset to
  the Chrome browser fingerprint — matches Chrome at the TLS, HTTP/2
  SETTINGS, and pseudo-header order levels. After each chain succeeds,
  cookies are copied into the long-lived stdlib jar.
- **Stdlib http.Client** (`transport_tls.go`) uses `http2.Transport` with a
  custom `DialTLSContext` backed by
  [`refraction-networking/utls`](https://github.com/refraction-networking/utls)
  — Chrome ClientHello over HTTP/2 for every `/api/*` request the ogen
  client makes.

If Polar ever tightens the WAF ruleset further, the standard escape hatches
are (1) bump the uTLS / azuretls dependency to track current Chrome, and (2)
fall back to manually pasting cookies from a browser into `polar-cookies.json`.
