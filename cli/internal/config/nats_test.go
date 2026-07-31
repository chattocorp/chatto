package config

import (
	"strings"
	"testing"

	"hmans.de/chatto/pkg/natsauth"
)

func TestChattoConfig_Validate_NATSClientTokenMatchesEmbedded(t *testing.T) {
	cfg := validTestConfig()
	cfg.NATS.Embedded = EmbeddedNATSConfig{
		Enabled:   true,
		Port:      4222,
		AuthToken: "embedded-token",
	}
	cfg.NATS.Client = NATSClientConfig{
		AuthMethod: natsauth.AuthToken,
		Token:      "other-token",
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "nats.client.token must match nats.embedded.auth_token") {
		t.Fatalf("Validate() error = %v, want NATS token mismatch", err)
	}
}

func TestEmbeddedNATSConfig_BindAddressOrDefault(t *testing.T) {
	tests := []struct {
		name        string
		bindAddress string
		want        string
	}{
		{
			name:        "empty returns localhost",
			bindAddress: "",
			want:        "127.0.0.1",
		},
		{
			name:        "custom value returned",
			bindAddress: "0.0.0.0",
			want:        "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &EmbeddedNATSConfig{BindAddress: tt.bindAddress}
			if got := c.BindAddressOrDefault(); got != tt.want {
				t.Errorf("BindAddressOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChattoConfig_Validate_EmbeddedNATS(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
			NATS: NATSConfig{
				Embedded: EmbeddedNATSConfig{
					Enabled:   true,
					Port:      4222,
					HTTPPort:  8222,
					DataDir:   "./data",
					AuthToken: "test-token",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config with TCP port and token",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "port 0 allowed (disables TCP listener)",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.Port = 0
				c.NATS.Embedded.AuthToken = "" // Token not required when TCP disabled
			},
			wantError: false,
		},
		{
			name: "http_port 0 allowed (disables monitoring)",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.HTTPPort = 0
			},
			wantError: false,
		},
		{
			name: "TCP port enabled without token fails",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.Port = 4222
				c.NATS.Embedded.AuthToken = ""
			},
			wantError: true,
			errorMsg:  "nats.embedded.auth_token is required when TCP port is enabled",
		},
		{
			name: "invalid port fails",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.Port = -1
			},
			wantError: true,
			errorMsg:  "nats.embedded.port must be between 0 and 65535",
		},
		{
			name: "invalid http_port fails",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.HTTPPort = 70000
			},
			wantError: true,
			errorMsg:  "nats.embedded.http_port must be between 0 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}
