package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/logging"
)

func TestRuntimeCreatesAccountWithReadYourWrites(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)

	account, err := runtime.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if account.ID == "" {
		t.Fatal("created account ID is empty")
	}
	if account.CreatedAt.IsZero() {
		t.Fatal("created account timestamp is zero")
	}
	if got, ok := runtime.Accounts.Get(account.ID); !ok || got != account {
		t.Fatalf("projected account = %+v, %v; want %+v, true", got, ok, account)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeReplaysAccountsAfterFullRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg)
	account, err := first.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatalf("create account before restart: %v", err)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg)
	if got := restarted.Accounts.Count(); got != 1 {
		t.Fatalf("replayed account count = %d, want 1", got)
	}
	if got, ok := restarted.Accounts.Get(account.ID); !ok || got != account {
		t.Fatalf("replayed account = %+v, %v; want %+v, true", got, ok, account)
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
}

func embeddedTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		NATS: config.NATSConfig{
			Embedded: config.EmbeddedNATSConfig{
				Enabled: true,
				DataDir: t.TempDir(),
			},
		},
	}
}

func startTestRuntime(
	t *testing.T,
	cfg config.Config,
) (*Runtime, context.CancelFunc, <-chan error) {
	t.Helper()
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	runtime, err := New(testContext(t), cfg, logger)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	runErrors := make(chan error, 1)
	go func() {
		runErrors <- runtime.Run(runContext)
	}()
	if err := runtime.WaitReady(testContext(t)); err != nil {
		cancel()
		<-runErrors
		runtime.Close()
		t.Fatalf("wait for runtime readiness: %v", err)
	}
	return runtime, cancel, runErrors
}

func stopTestRuntime(
	t *testing.T,
	runtime *Runtime,
	cancel context.CancelFunc,
	runErrors <-chan error,
) {
	t.Helper()
	cancel()
	select {
	case err := <-runErrors:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("runtime shutdown error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
