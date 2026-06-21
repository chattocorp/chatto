package runtimeunit

import (
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestRequireStandaloneNATSClientURL(t *testing.T) {
	t.Run("allows configured client URL", func(t *testing.T) {
		cfg := config.ChattoConfig{}
		cfg.NATS.Client.URL = "nats://127.0.0.1:4222"

		if err := RequireStandaloneNATSClientURL(cfg, "exporter"); err != nil {
			t.Fatalf("expected configured NATS client URL to be accepted: %v", err)
		}
	})

	t.Run("explains standalone requirement", func(t *testing.T) {
		err := RequireStandaloneNATSClientURL(config.ChattoConfig{}, "exporter")
		if err == nil {
			t.Fatal("expected missing NATS client URL to fail")
		}

		msg := err.Error()
		for _, want := range []string{"exporter", "[nats.client]", "nats.embedded.port"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("expected error %q to mention %q", msg, want)
			}
		}
	})
}
