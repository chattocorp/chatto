package logbridge

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

func TestSlogWritesThroughCharmLogger(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWithOptions(&output, log.Options{Level: log.InfoLevel}).WithPrefix("core.Test")

	bridged := Slog(logger)
	bridged.Debug("hidden by the level")
	bridged.Info("Projection startup complete", "messages", 3)

	got := output.String()
	for _, want := range []string{"core.Test", "Projection startup complete", "messages=3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "hidden by the level") {
		t.Fatalf("output %q contains a record below the logger level", got)
	}
}

func TestSlogKeepsNil(t *testing.T) {
	if Slog(nil) != nil {
		t.Fatal("Slog(nil) is not nil")
	}
}
