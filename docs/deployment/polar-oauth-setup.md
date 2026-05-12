# Polar OAuth Setup

Before you can use polar-flow-mcp, you need to register a developer application with
Polar. This gives you the `POLAR_CLIENT_ID` and `POLAR_CLIENT_SECRET` that the server
uses to exchange authorization codes for access tokens.

![Polar developer console app registration](../images/polar-app-registration.png)

---

## Prerequisites

- A Polar account at [flow.polar.com](https://flow.polar.com). The server will manage
  training targets for this account (and any other users who link via OAuth).
- Your deployment's public URL (e.g., `https://polar.example.com`) — you need this for
  the redirect URI.

---

## Step 1: Access the Polar developer console

Visit [https://admin.polaraccesslink.com](https://admin.polaraccesslink.com) and sign
in with your Polar account credentials.

This is the Polar AccessLink developer portal, separate from the main Polar Flow app.

---

## Step 2: Create a new application

On the developer console home page, click **Create new client** (or the equivalent
button in the current UI).

Fill in the application details:

| Field | Value |
|-------|-------|
| **Application name** | `polar-flow-mcp` (or your preferred name) |
| **Application description** | Optional — "MCP server for Claude integration" |
| **Redirect URI** | `https://<your-host>/oauth/callback` |

The redirect URI must match **exactly** — including the scheme (`https://`), domain,
and path (`/oauth/callback`). Any mismatch will cause the OAuth flow to fail with an
error from Polar.

---

## Step 3: Request the required scope

Polar AccessLink requires you to specify which scopes your application needs.

Request the following scope:

```
accesslink.read_all
```

This scope grants read access to all Polar AccessLink data, including the training
targets API used by polar-flow-mcp.

---

## Step 4: Save your credentials

After creating the application, Polar provides:

- **Client ID** — a public identifier for your app
- **Client Secret** — a private secret used to exchange authorization codes

Copy both values. The client secret is shown **once** — if you lose it, you must
regenerate it in the developer console (which does not affect existing tokens).

---

## Step 5: Configure polar-flow-mcp

Set the credentials in your `docker-compose.yml`:

```yaml
environment:
  POLAR_CLIENT_ID: "<your-client-id>"
  POLAR_CLIENT_SECRET: "<your-client-secret>"
```

The redirect URI registered with Polar must exactly match the URL your reverse proxy
exposes for `/oauth/callback`. If you change domains later, update the redirect URI in
the developer console.

---

## Verifying the setup

After starting polar-flow-mcp, visit `https://<your-host>/oauth/login` in your
browser. You should be redirected to the Polar authorization page at
`https://flow.polar.com/oauth2/authorization?...`.

If the redirect does not happen:

- Check `/readyz` to confirm `POLAR_CLIENT_ID` and `POLAR_CLIENT_SECRET` are loaded
  (these are not directly validated by `/readyz`, but missing env vars would cause the
  OAuth handlers to fail when the flow starts).
- Check the server logs for errors at the `/oauth/login` handler.
- Verify your reverse proxy is forwarding requests to polar-flow-mcp correctly.

After you authorize the application on Polar's page, you will be redirected to
`/oauth/callback`. A successful callback stores your encrypted token and returns a
confirmation message. You are now linked and can use the MCP tools.

---

## Token lifetime

Polar OAuth tokens **do not expire** unless you explicitly revoke them in the Polar app
(under Settings → Applications). There is no automatic expiry or refresh — once linked,
you remain linked until you revoke access.

If a token is revoked or becomes invalid, the MCP tools will return an error. The fix
is to visit `/oauth/login` again to re-authorize.

---

## Multiple users

Each user who visits `/oauth/login` links their own Polar account. Their identity comes
from the reverse proxy's `Remote-User` header (or your configured `IDENTITY_HEADER`).
Multiple users can share a single polar-flow-mcp instance — each has their own isolated
token in the database.
