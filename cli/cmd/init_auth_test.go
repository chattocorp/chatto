package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
)

func TestInitSSOConfiguration(t *testing.T) {
	t.Parallel()
	for _, passwordLogin := range []bool{true, false} {
		opts := defaultInitOptions()
		opts.ConfigPath = filepath.Join(t.TempDir(), "chatto.toml")
		opts.DirectLogin = passwordLogin
		opts.DirectRegistration = false
		opts.WithSSO, opts.WithSearch, opts.WithLiveKit = true, true, true
		opts.SSO.IssuerURL = "https://id.example.com/realms/community"
		opts.SSO.ClientID = "test-client"
		opts.SSO.ClientSecret = "private-client-secret"
		opts.SSO.Label = "Community SSO"
		opts.SSOAutoProvision = true
		var output bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&output)
		if err := writeInitialConfig(cmd, opts); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(opts.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		var cfg config.ChattoConfig
		if err := toml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
		if cfg.Auth.DirectLoginOrDefault() != passwordLogin || cfg.Auth.DirectRegistrationOrDefault() {
			t.Fatal("access settings were not preserved")
		}
		if len(cfg.Auth.Providers) != 1 {
			t.Fatal("expected one active SSO provider")
		}
		provider := cfg.Auth.Providers[0]
		if provider.ID != "sso" || provider.Type != "oidc" || provider.IssuerURL != opts.SSO.IssuerURL || provider.ClientID != opts.SSO.ClientID || provider.ClientSecret != opts.SSO.ClientSecret || provider.Label != opts.SSO.Label || !provider.AutoProvisionOrDefault() {
			t.Fatal("SSO provider fields were lost or left commented")
		}
		if !cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault() || !strings.Contains(string(data), "\n[auth.email_otp]\n") {
			t.Fatal("SSO must preserve active email OTP defaults")
		}
		if !cfg.Search.Enabled || !cfg.LiveKit.IsConfigured() {
			t.Fatal("SSO must compose with other integrations")
		}
		printInitNextSteps(cmd, opts)
		if strings.Contains(output.String()+initReview(opts, "4000"), opts.SSO.ClientSecret) || strings.Contains(output.String(), opts.SSO.ClientID) {
			t.Fatal("SSO credentials must not appear in terminal output")
		}
		if !passwordLogin && !strings.Contains(output.String(), "temporarily set auth.direct_login = true") {
			t.Fatal("SSO-only setup needs accurate first-run instructions")
		}
	}
}

func TestInitAuthValidation(t *testing.T) {
	t.Parallel()
	opts := defaultInitOptions()
	opts.DirectLogin = false
	if validateInitAuth(opts) == nil {
		t.Fatal("must retain a login method")
	}
	opts.WithSSO = true
	if validateInitAuth(opts) == nil {
		t.Fatal("SSO requires an issuer and client ID")
	}
	opts.SSO.IssuerURL = "https://id.example.com/realm"
	opts.SSO.ClientID = "client"
	if err := validateInitAuth(opts); err != nil {
		t.Fatal(err)
	}
	if got := initSSOCallback(opts, "4510"); got != "http://localhost:4000/auth/providers/sso/callback" {
		t.Fatalf("callback = %s", got)
	}
	opts.PublicURL = ""
	if got := initSSOCallback(opts, "4510"); got != "http://localhost:4510/auth/providers/sso/callback" {
		t.Fatalf("callback = %s", got)
	}
	for _, issuer := range []string{"", "https://user:secret@id.example.com", "https://id.example.com?token=secret", "ftp://id.example.com"} {
		if validateInitIssuer(issuer) == nil {
			t.Fatal("accepted invalid issuer")
		}
	}
}
