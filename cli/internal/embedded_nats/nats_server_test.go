package embedded_nats

import (
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestServerOptionsPreserveChattoPolicy(t *testing.T) {
	cfg := &config.EmbeddedNATSConfig{
		Port:        4333,
		BindAddress: "127.0.0.2",
		HTTPPort:    8333,
		DataDir:     "/var/lib/chatto/nats",
		AuthToken:   "test-token",
	}

	options := serverOptions(cfg)

	if !options.JetStream {
		t.Fatal("JetStream is disabled")
	}
	if options.StoreDir != cfg.DataDir {
		t.Fatalf("store directory = %q, want %q", options.StoreDir, cfg.DataDir)
	}
	if options.DontListen {
		t.Fatal("TCP listener is disabled")
	}
	if options.Host != cfg.BindAddress || options.Port != cfg.Port {
		t.Fatalf("client listener = %s:%d, want %s:%d", options.Host, options.Port, cfg.BindAddress, cfg.Port)
	}
	if options.Authorization != cfg.AuthToken {
		t.Fatalf("authorization token = %q, want configured token", options.Authorization)
	}
	if options.HTTPHost != cfg.BindAddress || options.HTTPPort != cfg.HTTPPort {
		t.Fatalf("monitor listener = %s:%d, want %s:%d", options.HTTPHost, options.HTTPPort, cfg.BindAddress, cfg.HTTPPort)
	}
}

func TestServerOptionsDisableTCPByDefault(t *testing.T) {
	options := serverOptions(&config.EmbeddedNATSConfig{
		DataDir: t.TempDir(),
	})

	if !options.DontListen {
		t.Fatal("TCP listener is enabled")
	}
	if options.HTTPPort != 0 {
		t.Fatalf("monitor port = %d, want disabled", options.HTTPPort)
	}
}
