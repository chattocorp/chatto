package migrations

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

func TestRunMigrationStep_SkipsMissingLegacySource(t *testing.T) {
	called := false
	err := runMigrationStep(
		context.Background(),
		nil,
		log.New(io.Discard),
		"missing_source",
		false,
		func() error {
			called = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("runMigrationStep returned error: %v", err)
	}
	if called {
		t.Fatal("runMigrationStep called a skipped step")
	}
}

func TestRunMigrationStep_WrapsStepError(t *testing.T) {
	stepErr := errors.New("boom")
	err := runMigrationStep(
		context.Background(),
		nil,
		log.New(io.Discard),
		"failing_step",
		true,
		func() error { return stepErr },
	)
	if !errors.Is(err, stepErr) {
		t.Fatalf("runMigrationStep error = %v, want wrapped %v", err, stepErr)
	}
	if !strings.Contains(err.Error(), "failing_step") {
		t.Fatalf("runMigrationStep error = %v, want step name", err)
	}
}

func TestMigrationUsageDelta(t *testing.T) {
	appended := (migrationEVTUsage{messages: 10, bytes: 100, ok: true}).delta(migrationEVTUsage{messages: 14, bytes: 175, ok: true})
	if appended.messages != 4 || appended.bytes != 75 {
		t.Fatalf("migrationEVTUsage.delta = (%d, %d), want (4, 75)", appended.messages, appended.bytes)
	}

	appended = (migrationEVTUsage{messages: 10, bytes: 100, ok: true}).delta(migrationEVTUsage{messages: 8, bytes: 90, ok: true})
	if appended.messages != 0 || appended.bytes != 0 {
		t.Fatalf("migrationEVTUsage.delta underflow = (%d, %d), want zeros", appended.messages, appended.bytes)
	}

	appended = (migrationEVTUsage{messages: 10, bytes: 100}).delta(migrationEVTUsage{messages: 14, bytes: 175, ok: true})
	if appended.messages != 0 || appended.bytes != 0 {
		t.Fatalf("migrationEVTUsage.delta without metrics = (%d, %d), want zeros", appended.messages, appended.bytes)
	}
}
