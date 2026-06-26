package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token "typ" discriminators. Every token the server signs carries one, and
// each consumer checks it, so a code can never be replayed as an access token.
const (
	typAccess  = "access"
	typRefresh = "refresh"
	typCode    = "code"
	typClient  = "client"
)

// authCodeTTL bounds how long an issued authorization code is exchangeable.
const authCodeTTL = 60 * time.Second

// nowUnix is the current Unix timestamp in seconds.
func nowUnix() int64 { return time.Now().Unix() }

// claims is the union of fields across all token types. Only the fields
// relevant to a given typ are populated.
type claims struct {
	Typ    string   `json:"typ"`
	Scope  string   `json:"scope,omitempty"`
	Groups []string `json:"groups,omitempty"`

	// Authorization-code fields.
	CodeChallenge string `json:"cc,omitempty"`
	CodeMethod    string `json:"ccm,omitempty"`
	RedirectURI   string `json:"ru,omitempty"`
	ClientID      string `json:"cid,omitempty"`

	// Registered-client fields (typ == client).
	RedirectURIs []string `json:"rus,omitempty"`

	jwt.RegisteredClaims
}

// sign serialises c as an EdDSA-signed compact JWS with the key id header.
func (a *Authenticator) sign(c *claims) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header["kid"] = a.kid
	return tok.SignedString(a.priv)
}

// parse verifies the signature (EdDSA only) and standard time claims, returning
// the decoded claims. It does not check typ — callers do.
func (a *Authenticator) parse(tokenStr string) (*claims, error) {
	var c claims
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"EdDSA"}))
	if _, err := parser.ParseWithClaims(tokenStr, &c, func(*jwt.Token) (any, error) {
		return a.pub, nil
	}); err != nil {
		return nil, err
	}
	return &c, nil
}

// mintAccess issues a short-lived access token bound to email and the resource.
func (a *Authenticator) mintAccess(email, scope string, groups []string) (string, time.Duration, error) {
	now := time.Now()
	c := &claims{
		Typ:    typAccess,
		Scope:  scope,
		Groups: groups,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    a.cfg.Issuer,
			Subject:   email,
			Audience:  jwt.ClaimStrings{a.cfg.Resource},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.cfg.AccessTTL)),
			ID:        newJTI(),
		},
	}
	s, err := a.sign(c)
	return s, a.cfg.AccessTTL, err
}

// mintRefresh issues a long-lived refresh token bound to email + client.
func (a *Authenticator) mintRefresh(email, clientID, scope string) (string, error) {
	now := time.Now()
	c := &claims{
		Typ:      typRefresh,
		Scope:    scope,
		ClientID: clientID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    a.cfg.Issuer,
			Subject:   email,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.cfg.RefreshTTL)),
			ID:        newJTI(),
		},
	}
	return a.sign(c)
}

// mintCode issues a PKCE-bound authorization code (typ == code).
func (a *Authenticator) mintCode(email, clientID, redirectURI, challenge, method, scope string) (string, error) {
	now := time.Now()
	c := &claims{
		Typ:           typCode,
		Scope:         scope,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		CodeChallenge: challenge,
		CodeMethod:    method,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    a.cfg.Issuer,
			Subject:   email,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(authCodeTTL)),
			ID:        newJTI(),
		},
	}
	return a.sign(c)
}

// mintClientID issues a stateless client_id: a signed token whose claims carry
// the registered redirect URIs. No storage is needed and it survives restarts.
func (a *Authenticator) mintClientID(redirectURIs []string) (string, error) {
	c := &claims{
		Typ:          typClient,
		RedirectURIs: redirectURIs,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   a.cfg.Issuer,
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}
	return a.sign(c)
}

// parseClientID verifies a client_id and returns its registered redirect URIs.
func (a *Authenticator) parseClientID(clientID string) (*claims, error) {
	c, err := a.parse(clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid client_id: %w", err)
	}
	if c.Typ != typClient {
		return nil, errors.New("client_id is not a client token")
	}
	return c, nil
}
