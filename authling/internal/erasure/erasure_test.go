package erasure

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/natsruntime"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/events"
)

func created(id, user, data string) *corev1.Event {
	return &corev1.Event{Id: "evt_created", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountCreated{AccountCreated: &corev1.AccountCreatedEvent{AccountId: id, UserKeyRef: user, CredentialKeyRef: data}}}
}
func requested(id, user, data string) *corev1.Event {
	return &corev1.Event{Id: "evt_request", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountErasureRequested{AccountErasureRequested: &corev1.AccountErasureRequestedEvent{AccountId: id, PriorCredentialEventId: "evt_created", UserKeyRef: user, CredentialKeyRef: data}}}
}
func released(id, request string) *corev1.Event {
	return &corev1.Event{Id: "evt_release", CreatedAt: timestamppb.Now(), Event: &corev1.Event_EmailReleased{EmailReleased: &corev1.EmailReleasedEvent{AccountId: id, ErasureRequestEventId: request}}}
}
func completed(id, request string) *corev1.Event {
	return &corev1.Event{Id: "evt_complete", CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountErased{AccountErased: &corev1.AccountErasedEvent{AccountId: id, ErasureRequestEventId: request}}}
}

func TestErasureIndexRejectsSubstitutionAndInvalidOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []*corev1.Event
	}{
		{"absent", []*corev1.Event{requested("acc_absent", "uk_a", "dk_a")}},
		{"other user key", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_b", "dk_a")}},
		{"other data key", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_b")}},
		{"duplicate request", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_a")}},
		{"release without request", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), released("acc_a", "evt_request")}},
		{"completion without release", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_a"), completed("acc_a", "evt_request")}},
		{"wrong correlation", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_a"), released("acc_a", "evt_wrong")}},
		{"mutation after request", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_a"), {Event: &corev1.Event_ProfileUpdated{ProfileUpdated: &corev1.ProfileUpdatedEvent{AccountId: "acc_a"}}}}},
		{"reuse keys", []*corev1.Event{created("acc_a", "uk_a", "dk_a"), requested("acc_a", "uk_a", "dk_a"), released("acc_a", "evt_request"), completed("acc_a", "evt_request"), created("acc_b", "uk_a", "dk_a")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProjection()
			for i, e := range tc.events {
				err := p.Apply(e, uint64(i+1))
				if i == len(tc.events)-1 {
					if err == nil {
						t.Fatal("invalid history accepted")
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Fail after user-key destruction, then let a new worker recover the data key.
type interruptedPurge struct {
	jetstream.KeyValue
	failed atomic.Bool
}

func (k *interruptedPurge) Purge(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if len(key) > 3 && key[:3] == "dk_" && k.failed.CompareAndSwap(false, true) {
		return errors.New("injected data-key purge failure")
	}
	return k.KeyValue.Purge(ctx, key, opts...)
}
func TestDurableWorkerResumesPartialDestructionAndDuplicateDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, stream, err := storage.Open(ctx, connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &interruptedPurge{KeyValue: stores.Keys}
	vault := keyvault.New(wrapped)
	_, user, data, key, err := vault.ProvisionCredentialKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clear(key)
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	pub := evtstream.NewPublisher(events.NewEncodedEventLog(js, stream, logger))
	// The index validates key ownership without decrypting protected fields.
	creation := created("acc_test", user, data)
	c := creation.GetAccountCreated()
	c.CredentialEnvelopeVersion = 1
	c.EmailNonce = []byte{1}
	c.EmailCiphertext = []byte{2}
	c.PasswordVerifierNonce = []byte{3}
	c.PasswordVerifierCiphertext = []byte{4}
	c.PreferredUsernameNonce = []byte{5}
	c.PreferredUsernameCiphertext = []byte{6}
	if _, err := pub.AppendAccountCreated(ctx, creation); err != nil {
		t.Fatal(err)
	}
	projection := NewProjection()
	handle := events.NewDecodedProjectionHandle(js, stream, projection, evtstream.Decode, logger)
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	runErr := make(chan error, 1)
	go func() { runErr <- handle.Projector().Run(runCtx) }()
	defer func() { stop(); <-runErr }()
	if err := handle.Projector().WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	tail, err := pub.AccountTail(ctx, "acc_test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pub.AppendAccountErasure(ctx, requested("acc_test", user, data), released("acc_test", "evt_request"), tail, 0); err != nil {
		t.Fatal(err)
	}
	service := New(pub, handle, vault)
	if err := service.ConfigureWorker(ctx, stream, logger, func(context.Context, State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := service.Complete(ctx, "acc_test"); err == nil {
		t.Fatal("purge failure not observed")
	}
	if _, err := stores.Keys.Get(ctx, user); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		t.Fatalf("user key remained: %v", err)
	}
	if _, err := stores.Keys.Get(ctx, data); err != nil {
		t.Fatal("data key should remain for retry", err)
	}
	// A fresh service shares the durable consumer and processes the unacked request.
	replacement := New(pub, handle, keyvault.New(stores.Keys))
	if err := replacement.ConfigureWorker(ctx, stream, logger, func(context.Context, State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerErr := make(chan error, 1)
	go func() { workerErr <- replacement.Run(workerCtx) }()
	defer func() { stopWorker(); <-workerErr }()
	for {
		state, _ := projection.Get("acc_test")
		if state.Complete {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := stores.Keys.Get(ctx, data); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		t.Fatalf("data key remained: %v", err)
	}
	tail, err = pub.AccountTail(ctx, "acc_test")
	if err != nil {
		t.Fatal(err)
	}
	// Retrying without knowledge of the successful completion must not append again.
	if err := replacement.Complete(ctx, "acc_test"); err != nil {
		t.Fatal(err)
	}
	later, err := pub.AccountTail(ctx, "acc_test")
	if err != nil || later != tail {
		t.Fatal("completion duplicated", err)
	}
}
