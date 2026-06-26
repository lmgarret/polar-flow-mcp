package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// OAuth endpoint paths, all served by this server (the app is its own AS).
const (
	// MetadataPath is the RFC 9728 protected-resource metadata location.
	MetadataPath = "/.well-known/oauth-protected-resource"
	// ASMetadataPath is the RFC 8414 authorization-server metadata location.
	ASMetadataPath = "/.well-known/oauth-authorization-server"
	// AuthorizePath is the browser consent endpoint (forward-auth protected).
	AuthorizePath = "/mcp/oauth/authorize"
	// TokenPath is the token endpoint (server-to-server).
	TokenPath = "/mcp/oauth/token"
	// RegisterPath is the RFC 7591 Dynamic Client Registration endpoint.
	RegisterPath = "/mcp/oauth/register"
)

// maxBody caps request bodies on the registration/token endpoints.
const maxBody = 1 << 20

// RegisterRoutes wires every OAuth endpoint except /mcp itself (which the caller
// wraps with ProtectedHandler). It registers both the bare well-known paths and
// the "/mcp"-suffixed variants some MCP clients probe.
func (a *Authenticator) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET "+MetadataPath, a.MetadataHandler())
	mux.Handle("GET "+MetadataPath+"/mcp", a.MetadataHandler())
	mux.Handle("GET "+ASMetadataPath, a.ASMetadataHandler())
	mux.Handle("GET "+ASMetadataPath+"/mcp", a.ASMetadataHandler())
	mux.Handle("POST "+RegisterPath, a.RegisterHandler())
	mux.Handle("GET "+AuthorizePath, a.AuthorizeHandler())
	mux.Handle("POST "+TokenPath, a.TokenHandler())
}

// ProtectedHandler wraps the MCP handler with the Origin guard and the access-
// token requirement.
func (a *Authenticator) ProtectedHandler(mcp http.Handler) http.Handler {
	return a.OriginGuard(a.Middleware(mcp))
}

// ASMetadataHandler serves RFC 8414 authorization-server metadata.
func (a *Authenticator) ASMetadataHandler() http.Handler {
	body := map[string]any{
		"issuer":                                a.cfg.Issuer,
		"authorization_endpoint":                a.authorizeURL(),
		"token_endpoint":                        a.tokenURL(),
		"registration_endpoint":                 a.registerURL(),
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"openid", "profile", "offline_access"},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, body)
	})
}

// registrationResponse is the RFC 7591 success body for a public PKCE client.
type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// RegisterHandler implements Dynamic Client Registration. Registration grants
// nothing on its own (a client_id alone yields no access); the gate is the
// forward-auth + allowlist on /authorize. Redirect URIs are restricted to the
// known Claude callbacks + loopback so a rogue registration cannot point codes
// at an attacker-controlled URL.
func (a *Authenticator) RegisterHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		if err := dec.Decode(&req); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "request body is not valid JSON")
			return
		}
		if len(req.RedirectURIs) == 0 {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
			return
		}
		for _, ru := range req.RedirectURIs {
			if !a.redirectAllowed(ru) {
				oauthError(w, http.StatusBadRequest, "invalid_redirect_uri",
					"redirect_uri not permitted: "+ru)
				return
			}
		}

		clientID, err := a.mintClientID(req.RedirectURIs)
		if err != nil {
			oauthError(w, http.StatusInternalServerError, "server_error", "could not issue client_id")
			return
		}
		writeJSON(w, http.StatusCreated, registrationResponse{
			ClientID:                clientID,
			ClientIDIssuedAt:        nowUnix(),
			RedirectURIs:            req.RedirectURIs,
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: "none",
		})
	})
}

// AuthorizeHandler is the browser consent endpoint. It must sit behind the
// forward-auth proxy: the proxy logs the user in and sets the identity header.
func (a *Authenticator) AuthorizeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		clientID := q.Get("client_id")
		redirectURI := q.Get("redirect_uri")
		state := q.Get("state")

		// Validate client + redirect_uri BEFORE redirecting anything back, so a
		// bad/forged redirect target never receives a code or an error.
		client, err := a.parseClientID(clientID)
		if err != nil {
			http.Error(w, "invalid client_id", http.StatusBadRequest)
			return
		}
		if redirectURI == "" || !contains(client.RedirectURIs, redirectURI) || !a.redirectAllowed(redirectURI) {
			http.Error(w, "redirect_uri is not registered for this client", http.StatusBadRequest)
			return
		}

		// From here, protocol errors are reported by redirecting back per OAuth.
		if q.Get("response_type") != "code" {
			http.Redirect(w, r, redirectWithError(redirectURI, state, "unsupported_response_type", "only code is supported"), http.StatusFound)
			return
		}
		challenge := q.Get("code_challenge")
		if challenge == "" || q.Get("code_challenge_method") != "S256" {
			http.Redirect(w, r, redirectWithError(redirectURI, state, "invalid_request", "PKCE with S256 is required"), http.StatusFound)
			return
		}
		if res := q.Get("resource"); res != "" && res != a.cfg.Resource {
			http.Redirect(w, r, redirectWithError(redirectURI, state, "invalid_target", "unknown resource"), http.StatusFound)
			return
		}

		// Identity comes from the forward-auth proxy, trusted only from a
		// trusted peer. If absent, the proxy is misconfigured (it should have
		// challenged the browser before this handler ran).
		id, ok := a.forwardAuthIdentity(r)
		if !ok {
			http.Error(w, "no authenticated identity; this endpoint must be served behind a forward-auth proxy", http.StatusUnauthorized)
			return
		}
		if !a.allowedEmail(id.Email) || !a.allowedGroups(id.Groups) {
			a.log.Warn("authorize denied: identity not on allowlist", "email", id.Email)
			http.Error(w, "this account is not permitted to use this connector", http.StatusForbidden)
			return
		}

		code, err := a.mintCode(id.Email, clientID, redirectURI, challenge, "S256", q.Get("scope"))
		if err != nil {
			http.Redirect(w, r, redirectWithError(redirectURI, state, "server_error", "could not issue code"), http.StatusFound)
			return
		}
		u, _ := url.Parse(redirectURI)
		rq := u.Query()
		rq.Set("code", code)
		if state != "" {
			rq.Set("state", state)
		}
		u.RawQuery = rq.Encode()
		a.log.Info("authorization granted", "email", id.Email)
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
}

// TokenHandler implements the token endpoint for the authorization_code and
// refresh_token grants. Public client (PKCE), so no client authentication.
func (a *Authenticator) TokenHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if err := r.ParseForm(); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_request", "could not parse form")
			return
		}
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			a.grantAuthCode(w, r)
		case "refresh_token":
			a.grantRefresh(w, r)
		default:
			oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type not supported")
		}
	})
}

// tokenResponse is the RFC 6749 success body.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

func (a *Authenticator) grantAuthCode(w http.ResponseWriter, r *http.Request) {
	f := r.PostForm
	code, redirectURI, clientID, verifier := f.Get("code"), f.Get("redirect_uri"), f.Get("client_id"), f.Get("code_verifier")
	if code == "" || verifier == "" || clientID == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code, client_id and code_verifier are required")
		return
	}
	c, err := a.parse(code)
	if err != nil || c.Typ != typCode {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid or expired")
		return
	}
	if c.ClientID != clientID || c.RedirectURI != redirectURI {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code does not match this client/redirect_uri")
		return
	}
	if !verifyPKCE(c.CodeChallenge, c.CodeMethod, verifier) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	if !a.allowedEmail(c.Subject) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "subject is no longer permitted")
		return
	}
	a.issueTokens(w, c.Subject, c.ClientID, c.Scope)
}

func (a *Authenticator) grantRefresh(w http.ResponseWriter, r *http.Request) {
	f := r.PostForm
	refresh, clientID := f.Get("refresh_token"), f.Get("client_id")
	if refresh == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	c, err := a.parse(refresh)
	if err != nil || c.Typ != typRefresh {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid or expired")
		return
	}
	if clientID != "" && c.ClientID != clientID {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token was issued to another client")
		return
	}
	if !a.allowedEmail(c.Subject) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "subject is no longer permitted")
		return
	}
	a.issueTokens(w, c.Subject, c.ClientID, c.Scope)
}

// issueTokens mints and writes a fresh access + (rotated) refresh token pair.
func (a *Authenticator) issueTokens(w http.ResponseWriter, email, clientID, scope string) {
	access, ttl, err := a.mintAccess(email, scope, nil)
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "could not issue access token")
		return
	}
	refresh, err := a.mintRefresh(email, clientID, scope)
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "could not issue refresh token")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(ttl.Seconds()),
		RefreshToken: refresh,
		Scope:        scope,
	})
}

// redirectAllowed reports whether raw is an acceptable redirect URI: a known
// Claude hosted callback, a loopback URL (native apps such as Claude Code), or
// an operator-configured extra. This bounds where authorization codes can land.
func (a *Authenticator) redirectAllowed(raw string) bool {
	for _, e := range a.cfg.ExtraRedirectURIs {
		if raw == e {
			return true
		}
	}
	switch raw {
	case "https://claude.ai/api/mcp/auth_callback", "https://claude.com/api/mcp/auth_callback":
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return false
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "127.0.0.1", "localhost", "::1":
			return true
		}
	}
	return false
}

// verifyPKCE checks a PKCE code_verifier against the stored challenge (S256).
func verifyPKCE(challenge, method, verifier string) bool {
	if method != "S256" || challenge == "" || verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(want), []byte(challenge)) == 1
}
