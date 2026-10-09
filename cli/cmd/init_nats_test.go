package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nats-io/nkeys"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

func TestInitExternalNATS(t *testing.T) {
	t.Parallel()
	pair, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	defer pair.Wipe()
	seed, err := pair.Seed()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []natsauth.AuthMethod{natsauth.AuthNone, natsauth.AuthToken, natsauth.AuthUserPass, natsauth.AuthCredentials, natsauth.AuthNKey} {
		t.Run(string(method), func(t *testing.T) {
			opts := defaultInitOptions()
			opts.ConfigPath = filepath.Join(t.TempDir(), "chatto.toml")
			opts.EmbeddedNATS = false
			opts.WithSearch, opts.WithLiveKit = true, true
			opts.NATSClient = config.NATSClientConfig{
				URL: "tls://nats.example.com:4222", AuthMethod: method,
				Token: "test-token", Username: "test-user", Password: "test-password",
				CredentialsFile: "./server.creds", NKeySeed: string(seed),
			}
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
			if cfg.NATS.Embedded.Enabled || cfg.NATS.Client.URL != opts.NATSClient.URL || cfg.NATS.Client.AuthMethod != method {
				t.Fatal("generated config does not select the external NATS server")
			}
			if !cfg.Search.Enabled || !cfg.SearchProvider.Enabled || !cfg.LiveKit.IsConfigured() {
				t.Fatal("external NATS must compose with search and LiveKit")
			}
			for _, field := range []struct {
				value, want string
				selected    bool
			}{
				{cfg.NATS.Client.Token, opts.NATSClient.Token, method == natsauth.AuthToken},
				{cfg.NATS.Client.Username, opts.NATSClient.Username, method == natsauth.AuthUserPass},
				{cfg.NATS.Client.Password, opts.NATSClient.Password, method == natsauth.AuthUserPass},
				{cfg.NATS.Client.CredentialsFile, opts.NATSClient.CredentialsFile, method == natsauth.AuthCredentials},
				{cfg.NATS.Client.NKeySeed, opts.NATSClient.NKeySeed, method == natsauth.AuthNKey},
			} {
				if field.selected && field.value != field.want || !field.selected && field.value != "" {
					t.Fatal("only the selected authentication settings must be active")
				}
				if strings.Contains(output.String()+initReview(opts, "4000"), field.want) {
					t.Fatal("NATS authentication settings leaked into output")
				}
			}
		})
	}
}

func TestInitNATSValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "https://example.com", "nats://user:secret@example.com", "nats://example.com/path", "nats://example.com:65536", "nats://example.com?token=secret"} {
		if validateInitNATSURL(value) == nil {
			t.Errorf("accepted invalid NATS URL %q", value)
		}
	}
	for _, value := range []string{"nats://localhost:4222", "tls://nats.example.com:4222", "nats://n1:4222, nats://n2:4222"} {
		if err := validateInitNATSURL(value); err != nil {
			t.Errorf("rejected valid NATS URL: %v", err)
		}
	}
	for _, method := range []natsauth.AuthMethod{natsauth.AuthToken, natsauth.AuthUserPass, natsauth.AuthCredentials, natsauth.AuthNKey, "invalid"} {
		if validateInitNATSClient(config.NATSClientConfig{URL: "nats://localhost:4222", AuthMethod: method}) == nil {
			t.Errorf("accepted missing authentication for %s", method)
		}
	}
}
