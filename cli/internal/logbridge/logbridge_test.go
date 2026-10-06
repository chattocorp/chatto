package logbridge

import (
	"bytes"
	"strings"
	"testing"
	"time"

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

// Chatto's JSON logs write durations as strings such as "1.5s". Records from
// the framework must keep that format.
func TestSlogKeepsDurationStringsInJSON(t *testing.T) {
	var output bytes.Buffer
	logger := log.NewWithOptions(&output, log.Options{Formatter: log.JSONFormatter})

	Slog(logger).Info("Projection startup complete", "duration", 1500*time.Millisecond)

	if got := output.String(); !strings.Contains(got, `"duration":"1.5s"`) {
		t.Fatalf("output %q does not contain the duration as a string", got)
	}
}
