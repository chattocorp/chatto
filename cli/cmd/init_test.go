package cmd

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/huh/v2"
	"hmans.de/chatto/internal/config"
)

func TestRunInitCommandCreatesEmbeddedConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	var output bytes.Buffer
	var gotWizardOptions initWizardOptions

	err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
		in:      strings.NewReader(""),
		out:     &output,
		entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32*5)),
		getenv:  func(string) string { return "" },
		wizard: func(answers *initAnswers, opts initWizardOptions) error {
			gotWizardOptions = opts
			answers.PublicURL = "https://chat.example.com"
			answers.ListenPort = "4444"
			answers.EmbeddedDataDir = "/var/lib/chatto"
			answers.Confirmed = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runInitCommand() error = %v", err)
	}
	if gotWizardOptions.configPath != configPath {
		t.Fatalf("wizard config path = %q, want %q", gotWizardOptions.configPath, configPath)
	}

	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if cfg.Webserver.URL != "https://chat.example.com" || cfg.Webserver.Port != 4444 {
		t.Fatalf("generated webserver = %q:%d", cfg.Webserver.URL, cfg.Webserver.Port)
	}
	assertHexSecret(t, "core secret", cfg.Core.SecretKey)
	assertHexSecret(t, "cookie signing secret", cfg.Webserver.CookieSigningSecret)
	assertHexSecret(t, "cookie encryption secret", cfg.Webserver.CookieEncryptionSecret)
	assertHexSecret(t, "asset signing secret", cfg.Core.Assets.SigningSecret)
	assertHexSecret(t, "embedded NATS token", cfg.NATS.Embedded.AuthToken)
	if !cfg.NATS.Embedded.Enabled || cfg.NATS.Embedded.DataDir != "/var/lib/chatto" {
		t.Fatalf("generated embedded NATS = %+v", cfg.NATS.Embedded)
	}
	if cfg.NATS.Embedded.Port != 0 {
		t.Fatalf("generated embedded NATS port = %d, want 0 when commented", cfg.NATS.Embedded.Port)
	}
	if cfg.NATS.Client.URL != "" {
		t.Fatalf("generated external NATS URL = %q, want empty", cfg.NATS.Client.URL)
	}
	if cfg.NATS.Replicas != 1 {
		t.Fatalf("generated NATS replicas = %d, want 1", cfg.NATS.Replicas)
	}
	if cfg.Core.Assets.StorageBackend != config.StorageBackendNATS {
		t.Fatalf("generated asset storage = %q", cfg.Core.Assets.StorageBackend)
	}
	if !cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault() {
		t.Fatal("generated config should enable email OTP throttling")
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat generated config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("generated config mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"allowed_origins = ['*']",
		"storage_backend = 'nats'",
		"[auth.email_otp]",
		"throttling_enabled = true",
		"# [[auth.providers]]",
		"# [nats.client]",
		"[nats.embedded]",
		"enabled = true",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated config missing %q", want)
		}
	}
	if !strings.Contains(output.String(), "The lights are on") || !strings.Contains(output.String(), "chatto run --config "+configPath) {
		t.Fatalf("completion output = %q", output.String())
	}
}

func TestRunInitCommandCreatesExternalNATSConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*initAnswers)
		assertNATS func(*testing.T, config.NATSClientConfig)
	}{
		{
			name: "credentials file",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthCredentials
				a.NATSCredentialsFile = "/run/secrets/chatto.creds"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.CredentialsFile != "/run/secrets/chatto.creds" {
					t.Fatalf("credentials file = %q", c.CredentialsFile)
				}
			},
		},
		{
			name: "token",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthToken
				a.NATSToken = "top-secret-token"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.Token != "top-secret-token" {
					t.Fatalf("token = %q", c.Token)
				}
			},
		},
		{
			name: "userpass",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthUserPass
				a.NATSUsername = "chatto"
				a.NATSPassword = "password"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.Username != "chatto" || c.Password != "password" {
					t.Fatalf("userpass = %q/%q", c.Username, c.Password)
				}
			},
		},
		{
			name: "nkey",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthNKey
				a.NATSNKeySeed = "SUABC"
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {
				if c.NKeySeed != "SUABC" {
					t.Fatalf("NKey seed = %q", c.NKeySeed)
				}
			},
		},
		{
			name: "none",
			configure: func(a *initAnswers) {
				a.NATSAuthMethod = config.NATSAuthNone
			},
			assertNATS: func(t *testing.T, c config.NATSClientConfig) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "chatto.toml")
			err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
				in:      strings.NewReader(""),
				out:     ioDiscard{},
				entropy: bytes.NewReader(bytes.Repeat([]byte{0x24}, 32*5)),
				getenv:  func(string) string { return "" },
				wizard: func(answers *initAnswers, _ initWizardOptions) error {
					answers.NATSMode = initNATSExternal
					answers.ExternalNATSURL = "nats://nats-1:4222,nats://nats-2:4222"
					answers.NATSReplicas = 3
					tt.configure(answers)
					answers.Confirmed = true
					return nil
				},
			})
			if err != nil {
				t.Fatalf("runInitCommand() error = %v", err)
			}
			cfg, err := config.ReadConfig(configPath)
			if err != nil {
				t.Fatalf("read generated config: %v", err)
			}
			if cfg.NATS.Embedded.Enabled {
				t.Fatal("embedded NATS should be disabled")
			}
			if cfg.NATS.Client.URL != "nats://nats-1:4222,nats://nats-2:4222" || cfg.NATS.Replicas != 3 {
				t.Fatalf("external NATS = %+v", cfg.NATS)
			}
			if cfg.NATS.Client.AuthMethod != ttAuthMethod(tt.name) {
				t.Fatalf("auth method = %q", cfg.NATS.Client.AuthMethod)
			}
			tt.assertNATS(t, cfg.NATS.Client)

			raw, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read generated config: %v", err)
			}
			if !strings.Contains(string(raw), "\n[nats.client]\n") || strings.Contains(string(raw), "\n# [nats.client]\n") {
				t.Fatalf("external NATS client table was not activated:\n%s", raw)
			}
		})
	}
}

func ttAuthMethod(name string) config.NATSAuthMethod {
	switch name {
	case "credentials file":
		return config.NATSAuthCredentials
	case "token":
		return config.NATSAuthToken
	case "userpass":
		return config.NATSAuthUserPass
	case "nkey":
		return config.NATSAuthNKey
	default:
		return config.NATSAuthNone
	}
}

func TestRunInitCommandCancellationWritesNothing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	for _, wizard := range []func(*initAnswers, initWizardOptions) error{
		func(*initAnswers, initWizardOptions) error { return huh.ErrUserAborted },
		func(answers *initAnswers, _ initWizardOptions) error {
			answers.Confirmed = false
			return nil
		},
	} {
		err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
			in:      strings.NewReader(""),
			out:     ioDiscard{},
			entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32*5)),
			getenv:  func(string) string { return "" },
			wizard:  wizard,
		})
		if err == nil || !strings.Contains(err.Error(), "nothing was written") {
			t.Fatalf("runInitCommand() error = %v", err)
		}
		if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("config exists after cancellation: %v", err)
		}
	}
}

func TestRunInitCommandRefusesOverwriteBeforeWizard(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	if err := os.WriteFile(configPath, []byte("sentinel"), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}
	wizardCalled := false
	err := runInitCommand(initCommandOptions{configPath: configPath}, initCommandDependencies{
		wizard: func(*initAnswers, initWizardOptions) error {
			wizardCalled = true
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("runInitCommand() error = %v", err)
	}
	if wizardCalled {
		t.Fatal("wizard ran before overwrite protection")
	}
	got, err := os.ReadFile(configPath)
	if err != nil || string(got) != "sentinel" {
		t.Fatalf("existing config changed: %q, %v", got, err)
	}
}

func TestRunInitCommandAccessibleMode(t *testing.T) {
	tests := []struct {
		name     string
		flag     bool
		env      string
		wantMode bool
	}{
		{name: "default"},
		{name: "flag", flag: true, wantMode: true},
		{name: "environment", env: "1", wantMode: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "chatto.toml")
			gotAccessible := false
			err := runInitCommand(initCommandOptions{configPath: configPath, accessible: tt.flag}, initCommandDependencies{
				in:      strings.NewReader(""),
				out:     ioDiscard{},
				entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32*5)),
				getenv:  func(string) string { return tt.env },
				wizard: func(answers *initAnswers, opts initWizardOptions) error {
					gotAccessible = opts.accessible
					answers.Confirmed = true
					return nil
				},
			})
			if err != nil {
				t.Fatalf("runInitCommand() error = %v", err)
			}
			if gotAccessible != tt.wantMode {
				t.Fatalf("accessible = %v, want %v", gotAccessible, tt.wantMode)
			}
		})
	}
}

func TestInitWizardAccessibleReviewIncludesSummary(t *testing.T) {
	answers := defaultInitAnswers()
	answers.PublicURL = "https://chat.example.com"
	var output bytes.Buffer
	opts := initWizardOptions{
		input:      strings.NewReader("y\n"),
		output:     &output,
		accessible: true,
		configPath: "/etc/chatto/chatto.toml",
	}
	err := newInitForm(opts, initReviewGroup(&answers, opts.configPath, false)).Run()
	if err != nil {
		t.Fatalf("review form error = %v", err)
	}
	text := output.String()
	for _, want := range []string{"Launch card", "https://chat.example.com", "/etc/chatto/chatto.toml"} {
		if !strings.Contains(text, want) {
			t.Errorf("accessible output missing %q:\n%s", want, text)
		}
	}
}

func TestInitWizardValidators(t *testing.T) {
	valid := []struct {
		name string
		fn   func(string) error
		text string
	}{
		{name: "https URL", fn: validatePublicURL, text: "https://chat.example.com"},
		{name: "loopback URL", fn: validatePublicURL, text: "http://localhost:4000"},
		{name: "port", fn: validatePort, text: "4000"},
		{name: "NATS URL", fn: validateNATSURLs, text: "nats://one:4222,tls://two:4222"},
		{name: "not blank", fn: validateNotBlank("value"), text: "hello"},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(tt.text); err != nil {
				t.Fatalf("validate(%q) = %v", tt.text, err)
			}
		})
	}

	invalid := []struct {
		name string
		fn   func(string) error
		text string
	}{
		{name: "relative URL", fn: validatePublicURL, text: "/chat"},
		{name: "URL path", fn: validatePublicURL, text: "https://example.com/chat"},
		{name: "bad port", fn: validatePort, text: "70000"},
		{name: "bad NATS scheme", fn: validateNATSURLs, text: "https://nats.example.com"},
		{name: "blank", fn: validateNotBlank("value"), text: "  "},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(tt.text); err == nil {
				t.Fatalf("validate(%q) succeeded", tt.text)
			}
		})
	}
}

func assertHexSecret(t *testing.T, name, value string) {
	t.Helper()
	if len(value) != 64 {
		t.Fatalf("%s length = %d, want 64", name, len(value))
	}
	if _, err := hex.DecodeString(value); err != nil {
		t.Fatalf("%s is not hex: %v", name, err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
