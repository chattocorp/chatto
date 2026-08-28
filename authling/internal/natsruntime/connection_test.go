package natsruntime

import (
	"testing"

	"hmans.de/authling/internal/config"
)

func TestOpenEmbeddedNATS(t *testing.T) {
	connection, err := Open(t.Context(), config.NATSConfig{
		Embedded: config.EmbeddedNATSConfig{
			Enabled: true,
			DataDir: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("open embedded NATS: %v", err)
	}
	if !connection.NATS.IsConnected() {
		t.Fatal("embedded NATS client is not connected")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close embedded NATS: %v", err)
	}
}
