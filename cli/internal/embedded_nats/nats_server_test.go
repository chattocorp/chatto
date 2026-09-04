package embedded_nats

import (
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

func TestServerOptionsPreserveChattoPolicy(t *testing.T) {
	cfg := &config.EmbeddedNATSConfig{
		Port:         4333,
		BindAddress:  "127.0.0.2",
		HTTPPort:     8333,
		DataDir:      "/var/lib/chatto/nats",
		SyncInterval: "always",
		AuthToken:    "test-token",
	}

	options, err := serverOptions(cfg)
	if err != nil {
		t.Fatalf("serverOptions() error = %v", err)
	}

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
	if !options.SyncAlways {
		t.Fatal("sync always is disabled")
	}
}

func TestServerOptionsDisableTCPByDefault(t *testing.T) {
	options, err := serverOptions(&config.EmbeddedNATSConfig{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("serverOptions() error = %v", err)
	}

	if !options.DontListen {
		t.Fatal("TCP listener is enabled")
	}
	if options.HTTPPort != 0 {
		t.Fatalf("monitor port = %d, want disabled", options.HTTPPort)
	}
	if options.SyncInterval != 0 || options.SyncAlways {
		t.Fatalf("sync settings = (%s, %t), want NATS defaults", options.SyncInterval, options.SyncAlways)
	}
}

func TestServerOptionsSetSyncInterval(t *testing.T) {
	options, err := serverOptions(&config.EmbeddedNATSConfig{
		DataDir:      t.TempDir(),
		SyncInterval: "30s",
	})
	if err != nil {
		t.Fatalf("serverOptions() error = %v", err)
	}
	if options.SyncInterval != 30*time.Second || options.SyncAlways {
		t.Fatalf("sync settings = (%s, %t), want (30s, false)", options.SyncInterval, options.SyncAlways)
	}
}
