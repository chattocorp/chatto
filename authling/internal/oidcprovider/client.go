// Package oidcprovider implements Authling's OpenID Connect provider boundary.
package oidcprovider

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/url"
	"strings"
	"time"

	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"hmans.de/authling/internal/config"
)

const consentPath = "/oidc/consent"

// ClientSource identifies how Authling learned a client's metadata.
type ClientSource string

const (
	ClientSourceConfigured ClientSource = "configured"
	ClientSourceCIMD       ClientSource = "cimd"
)

// Client is the immutable metadata snapshot used for one protocol operation.
type Client struct {
	IDValue     string
	NameValue   string
	DisplayHost string
	Redirects   []string
	Method      liboidc.AuthMethod
	Secret      string
	Source      ClientSource
	Development bool
}

func (c *Client) GetID() string                     { return c.IDValue }
func (c *Client) RedirectURIs() []string            { return append([]string(nil), c.Redirects...) }
func (*Client) PostLogoutRedirectURIs() []string    { return nil }
func (*Client) ApplicationType() op.ApplicationType { return op.ApplicationTypeWeb }
func (c *Client) AuthMethod() liboidc.AuthMethod    { return c.Method }
func (*Client) ResponseTypes() []liboidc.ResponseType {
	return []liboidc.ResponseType{liboidc.ResponseTypeCode}
}
func (*Client) GrantTypes() []liboidc.GrantType      { return []liboidc.GrantType{liboidc.GrantTypeCode} }
func (*Client) AccessTokenType() op.AccessTokenType  { return op.AccessTokenTypeBearer }
func (*Client) IDTokenLifetime() time.Duration       { return 5 * time.Minute }
func (c *Client) DevMode() bool                      { return c.Development }
func (*Client) IDTokenUserinfoClaimsAssertion() bool { return false }
func (*Client) ClockSkew() time.Duration             { return 0 }
func (*Client) IsScopeAllowed(scope string) bool     { return scope == liboidc.ScopeOpenID }
func (*Client) RestrictAdditionalIdTokenScopes() func([]string) []string {
	return func([]string) []string { return nil }
}
func (*Client) RestrictAdditionalAccessTokenScopes() func([]string) []string {
	return func([]string) []string { return nil }
}
func (*Client) LoginURL(requestID string) string {
	return consentPath + "?id=" + url.QueryEscape(requestID)
}

// Resolver resolves conventional configuration and CIMD URL client IDs into
// the same protocol client representation.
type Resolver struct {
	configured map[string]*Client
	cimd       *CIMDResolver
}

// NewResolver constructs the combined client resolver.
func NewResolver(cfg config.Config, cimd *CIMDResolver) *Resolver {
	configured := make(map[string]*Client, len(cfg.OIDC.Clients))
	development := strings.HasPrefix(cfg.HTTP.PublicURLOrDefault(), "http://")
	for _, declared := range cfg.OIDC.Clients {
		method := liboidc.AuthMethodNone
		if declared.Secret != "" {
			method = liboidc.AuthMethodBasic
		}
		configured[declared.ID] = &Client{
			IDValue: declared.ID, NameValue: strings.TrimSpace(declared.Name), DisplayHost: "configured by this Authling operator",
			Redirects: append([]string(nil), declared.RedirectURIs...), Method: method,
			Secret: declared.Secret, Source: ClientSourceConfigured, Development: development,
		}
	}
	return &Resolver{configured: configured, cimd: cimd}
}

// Resolve resolves one exact client identifier.
func (r *Resolver) Resolve(ctx context.Context, clientID string) (*Client, error) {
	if client, ok := r.configured[clientID]; ok {
		copy := *client
		copy.Redirects = append([]string(nil), client.Redirects...)
		return &copy, nil
	}
	if strings.HasPrefix(strings.ToLower(clientID), "https://") && r.cimd != nil {
		return r.cimd.Resolve(ctx, clientID)
	}
	return nil, clientNotFoundError{clientID: clientID}
}

// AuthorizeSecret performs constant-time verification for a conventional
// confidential client's configured secret.
func (r *Resolver) AuthorizeSecret(ctx context.Context, clientID, candidate string) error {
	client, err := r.Resolve(ctx, clientID)
	if err != nil || client.Source != ClientSourceConfigured || client.Method != liboidc.AuthMethodBasic {
		return fmt.Errorf("invalid client credentials")
	}
	want, got := sha256.Sum256([]byte(client.Secret)), sha256.Sum256([]byte(candidate))
	if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		return fmt.Errorf("invalid client credentials")
	}
	return nil
}

type clientNotFoundError struct{ clientID string }

func (e clientNotFoundError) Error() string { return "OIDC client not found" }
func (clientNotFoundError) IsNotFound()     {}
