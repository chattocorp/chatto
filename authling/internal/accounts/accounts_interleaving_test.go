package accounts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/events"
)

func TestCredentialBoundAuditRequestsWaitForEmailClaim(t *testing.T) {
	ctx := accountTestContext(t)
	connection, err := natsruntime.Open(ctx, config.NATSConfig{
		Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("open NATS: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
		}
	})
	js, stream, err := storage.Open(ctx, connection.NATS, 1)
	if err != nil {
		t.Fatalf("open event storage: %v", err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatalf("open stores: %v", err)
	}
	vault := keyvault.New(stores.Keys)
	indexKey, err := vault.WorkflowKey(ctx)
	if err != nil {
		t.Fatalf("open workflow key: %v", err)
	}
	defer clear(indexKey)
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	publisher := evtstream.NewPublisher(events.NewEncodedEventLog(js, stream, logger))
	projection := NewProjection(vault, indexKey)
	handle := events.NewDecodedProjectionHandle(js, stream, projection, evtstream.Decode, logger)
	service, err := NewService(ctx, publisher, handle, vault, 12)
	if err != nil {
		t.Fatalf("create account service: %v", err)
	}
	runCancel, runErrors := runAccountTestProjector(t, handle.Projector())

	const (
		oldEmail = "before@example.com"
		newEmail = "after@example.com"
		password = "a deliberately long test password"
	)
	account, err := service.CreateLocal(ctx, oldEmail, password)
	if err != nil {
		t.Fatalf("create local account: %v", err)
	}
	target, err := service.PrepareEmailChange(ctx, account.ID, password, newEmail)
	if err != nil {
		t.Fatalf("prepare email change: %v", err)
	}
	target, err = service.RecordEmailChangeRequested(ctx, target)
	if err != nil {
		t.Fatalf("record initial email-change request: %v", err)
	}

	claimStarted := make(chan struct{})
	releaseClaim := make(chan struct{})
	projection.Lock()
	projection.beforeEmailClaimApply = func() {
		close(claimStarted)
		<-releaseClaim
	}
	projection.Unlock()
	changeErrors := make(chan error, 1)
	go func() {
		_, err := service.ChangeEmail(ctx, target, newEmail)
		changeErrors <- err
	}()
	select {
	case <-claimStarted:
	case <-ctx.Done():
		t.Fatalf("wait for staged email change: %v", ctx.Err())
	}

	stagedTail, err := publisher.AccountTail(ctx, account.ID)
	if err != nil {
		t.Fatalf("read staged account tail: %v", err)
	}
	assertNoAuditAppend := func(name string, command func(context.Context) error) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			commandCtx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			if err := command(commandCtx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("command error = %v, want context deadline while registry apply is blocked", err)
			}
			tail, err := publisher.AccountTail(accountTestContext(t), account.ID)
			if err != nil {
				t.Fatalf("read account tail: %v", err)
			}
			if tail != stagedTail {
				t.Fatalf("account tail = %d, want staged email-change tail %d; audit event committed inside split batch", tail, stagedTail)
			}
		})
	}
	assertNoAuditAppend("email change request", func(commandCtx context.Context) error {
		_, err := service.RecordEmailChangeRequested(commandCtx, EmailChangeTarget{
			AccountID: account.ID, CredentialEventID: target.CredentialEventID,
		})
		return err
	})
	assertNoAuditAppend("password reset request", func(commandCtx context.Context) error {
		_, _, err := service.RecordPasswordResetRequested(commandCtx, oldEmail)
		return err
	})

	close(releaseClaim)
	select {
	case err := <-changeErrors:
		if err != nil {
			t.Fatalf("complete email change: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("wait for email change: %v", ctx.Err())
	}
	if err := handle.Projector().Err(); err != nil {
		t.Fatalf("live projection failed after interleaving: %v", err)
	}
	if _, ok, err := service.RecordPasswordResetRequested(ctx, newEmail); err != nil || !ok {
		t.Fatalf("record request against activated credential: ok=%v err=%v", ok, err)
	}

	stopAccountTestProjector(t, runCancel, runErrors)
	replayed := NewProjection(vault, indexKey)
	replayHandle := events.NewDecodedProjectionHandle(js, stream, replayed, evtstream.Decode, logger)
	replayCancel, replayErrors := runAccountTestProjector(t, replayHandle.Projector())
	if !replayed.HasEmail(newEmail) || replayed.HasEmail(oldEmail) {
		t.Fatalf("cold replay email registry: old=%v new=%v", replayed.HasEmail(oldEmail), replayed.HasEmail(newEmail))
	}
	stopAccountTestProjector(t, replayCancel, replayErrors)
}

func runAccountTestProjector(t *testing.T, projector *events.Projector) (context.CancelFunc, <-chan error) {
	t.Helper()
	runContext, cancel := context.WithCancel(context.Background())
	runErrors := make(chan error, 1)
	go func() { runErrors <- projector.Run(runContext) }()
	if err := projector.WaitForStartup(accountTestContext(t)); err != nil {
		cancel()
		<-runErrors
		t.Fatalf("wait for projector startup: %v", err)
	}
	return cancel, runErrors
}

func stopAccountTestProjector(t *testing.T, cancel context.CancelFunc, runErrors <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-runErrors:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("projector shutdown error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projector did not stop")
	}
}

func accountTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
