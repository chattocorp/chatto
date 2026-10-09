package cmd

import (
	"errors"
	"net/url"
	"strings"

	"hmans.de/chatto/internal/config"
)

func validateInitIssuer(value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("enter the HTTP or HTTPS issuer URL without credentials, a query, or a fragment")
	}
	return nil
}

func validateInitAuth(opts initOptions) error {
	if !opts.DirectLogin && !opts.WithSSO {
		return errors.New("enable password login or configure SSO on the Access page")
	}
	if !opts.WithSSO {
		return nil
	}
	if err := validateInitIssuer(opts.SSO.IssuerURL); err != nil {
		return err
	}
	if strings.TrimSpace(opts.SSO.ClientID) == "" {
		return errors.New("enter the SSO client ID")
	}
	return nil
}

// initSSOCallback uses the fixed local provider ID written by this wizard.
// The ID must remain stable after accounts link their provider identities.
func initSSOCallback(opts initOptions, port string) string {
	return initPublicURL(opts.PublicURL, port) + "/auth/providers/sso/callback"
}

// initAuthConfig activates the provider fields that normal config examples
// comment out. It excludes an abandoned SSO draft when the user switches SSO off.
func initAuthConfig(opts initOptions, base config.AuthConfig) any {
	if !opts.WithSSO {
		return base
	}
	type provider struct {
		ID            string `toml:"id" comment:"Stable provider ID. Do not change after accounts link SSO."`
		Type          string `toml:"type"`
		Label         string `toml:"label"`
		IssuerURL     string `toml:"issuer_url"`
		ClientID      string `toml:"client_id"`
		ClientSecret  string `toml:"client_secret" comment:"Keep this client secret private."`
		AutoProvision bool   `toml:"auto_provision" comment:"Allow new SSO identities to create an account after confirmation."`
	}
	return struct {
		config.AuthConfig
		Providers []provider `toml:"providers"`
	}{base, []provider{{
		ID: "sso", Type: config.AuthProviderTypeOpenIDConnect,
		Label: opts.SSO.Label, IssuerURL: opts.SSO.IssuerURL,
		ClientID: opts.SSO.ClientID, ClientSecret: opts.SSO.ClientSecret,
		AutoProvision: opts.SSOAutoProvision,
	}}}
}
