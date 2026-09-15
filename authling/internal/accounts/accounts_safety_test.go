package accounts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/natsruntime"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/events"
)

// publicationFault preserves the real publisher except at the fault boundary.
type publicationFault struct {
	*evtstream.Publisher
	appendAccount func(context.Context, *corev1.Event, *corev1.Event, uint64) (events.StreamPosition, error)
	registryError error
}

func (p publicationFault) AppendRegisteredAccount(ctx context.Context, account, claim *corev1.Event, tail uint64) (events.StreamPosition, error) {
	return p.appendAccount(ctx, account, claim, tail)
}
func (p publicationFault) AccountRegistryTail(ctx context.Context) (uint64, error) {
	if p.registryError != nil {
		return 0, p.registryError
	}
	return p.Publisher.AccountRegistryTail(ctx)
}

type safetyFixture struct {
	service   *Service
	publisher *evtstream.Publisher
	keys      jetstream.KeyValue
	js        jetstream.JetStream
	stream    jetstream.Stream
	stop      func()
}

func newSafetyFixture(t *testing.T, dataDir string, wrap func(jetstream.KeyValue) jetstream.KeyValue) safetyFixture {
	t.Helper()
	ctx := accountTestContext(t)
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: dataDir}})
	if err != nil {
		t.Fatal(err)
	}
	// Register connection cleanup first so replica projectors stop before storage.
	var closeOnce sync.Once
	closeConnection := func() {
		closeOnce.Do(func() {
			if err := connection.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(closeConnection)
	js, stream, err := storage.Open(ctx, connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}
	keys := stores.Keys
	if wrap != nil {
		keys = wrap(keys)
	}
	service, publisher, stopReplica := newSafetyReplica(t, js, stream, keys)
	return safetyFixture{service, publisher, stores.Keys, js, stream, func() { stopReplica(); closeConnection() }}
}
func newSafetyReplica(t *testing.T, js jetstream.JetStream, stream jetstream.Stream, keys jetstream.KeyValue) (*Service, *evtstream.Publisher, func()) {
	t.Helper()
	ctx := accountTestContext(t)
	vault := keyvault.New(keys)
	key, err := vault.WorkflowKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	publisher := evtstream.NewPublisher(events.NewEncodedEventLog(js, stream, logger))
	handle := events.NewDecodedProjectionHandle(js, stream, NewProjection(vault, key), evtstream.Decode, logger)
	service, err := NewService(ctx, publisher, handle, vault, 12)
	if err != nil {
		t.Fatal(err)
	}
	cancel, errs := runAccountTestProjector(t, handle.Projector())
	var once sync.Once
	stop := func() { once.Do(func() { stopAccountTestProjector(t, cancel, errs) }) }
	t.Cleanup(stop)
	return service, publisher, stop
}

func TestSignupPreservesKeysAfterUnknownPublicationOutcome(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "reply lost before observed commit", true: "reply lost after commit"}[committed], func(t *testing.T) {
			dataDir := t.TempDir()
			f := newSafetyFixture(t, dataDir, nil)
			ctx := accountTestContext(t)
			var created *corev1.AccountCreatedEvent
			f.service.publisher = publicationFault{Publisher: f.publisher, appendAccount: func(ctx context.Context, a, c *corev1.Event, tail uint64) (events.StreamPosition, error) {
				created = a.GetAccountCreated()
				if committed {
					if _, err := f.publisher.AppendRegisteredAccount(ctx, a, c, tail); err != nil {
						t.Fatal(err)
					}
				}
				return events.StreamPosition{}, context.DeadlineExceeded
			}}
			const password = "a long signup safety password"
			if _, err := f.service.CreateLocal(ctx, "signup@example.invalid", password); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("signup error = %v", err)
			}
			for _, ref := range []string{created.GetUserKeyRef(), created.GetCredentialKeyRef()} {
				if _, err := f.keys.Get(ctx, ref); err != nil {
					t.Fatalf("key lost after uncertain publication: %v", err)
				}
			}
			if got := countProvisioningMarkers(t, f.keys); got != 1 {
				t.Fatalf("markers = %d, want 1", got)
			}
			// Restart the embedded server as well as the projections.
			f.stop()
			restarted := newSafetyFixture(t, dataDir, nil)
			if committed {
				if _, err := restarted.service.AuthenticateLocal(accountTestContext(t), "signup@example.invalid", password); err != nil {
					t.Fatalf("login after restart: %v", err)
				}
				if _, err := restarted.service.CreateLocal(accountTestContext(t), "signup@example.invalid", password); !errors.Is(err, ErrEmailClaimed) {
					t.Fatalf("retry error = %v", err)
				}
			} else if restarted.service.Count() != 0 {
				t.Fatal("uncommitted signup became an account")
			}
			if got := countProvisioningMarkers(t, restarted.keys); got != 1 {
				t.Fatalf("restart lost provisioning marker: %d", got)
			}
		})
	}
}

func TestSignupCompensatesOnlyDefiniteFailures(t *testing.T) {
	for _, mode := range []string{"before publication", "conflict", "conflict then success"} {
		t.Run(mode, func(t *testing.T) {
			f := newSafetyFixture(t, t.TempDir(), nil)
			ctx := accountTestContext(t)
			baseline, err := f.keys.Keys(ctx)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			fault := publicationFault{Publisher: f.publisher, appendAccount: func(ctx context.Context, a, c *corev1.Event, tail uint64) (events.StreamPosition, error) {
				calls++
				if mode == "conflict then success" && calls > 1 {
					return f.publisher.AppendRegisteredAccount(ctx, a, c, tail)
				}
				return events.StreamPosition{}, events.ErrConflict
			}}
			if mode == "before publication" {
				fault.registryError = errors.New("registry unavailable")
			}
			f.service.publisher = fault
			_, err = f.service.CreateLocal(ctx, "signup@example.invalid", "a long signup safety password")
			if mode == "conflict then success" {
				if err != nil || calls != 2 {
					t.Fatalf("retry: calls=%d err=%v", calls, err)
				}
			} else {
				if err == nil {
					t.Fatal("expected signup failure")
				}
				keys, keyErr := f.keys.Keys(ctx)
				if keyErr != nil || len(keys) != len(baseline) {
					t.Fatalf("failed signup retained keys: count=%d err=%v", len(keys), keyErr)
				}
			}
			if got := countProvisioningMarkers(t, f.keys); got != 0 {
				t.Fatalf("markers = %d, want 0", got)
			}
		})
	}
}
func countProvisioningMarkers(t *testing.T, keys jetstream.KeyValue) int {
	t.Helper()
	names, err := keys.Keys(accountTestContext(t))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, name := range names {
		if strings.HasPrefix(name, "op_") {
			count++
		}
	}
	return count
}

// Only the marked login triggers the one-shot barrier. Projection and second
// replica key reads use normal contexts and do not run the callback.
type loginReadContextKey struct{}
type loginKeyBarrier struct {
	jetstream.KeyValue
	mu   sync.Mutex
	hook func()
}

func (b *loginKeyBarrier) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if ctx.Value(loginReadContextKey{}) != nil && strings.HasPrefix(key, "dk_") {
		b.mu.Lock()
		hook := b.hook
		b.hook = nil
		b.mu.Unlock()
		if hook != nil {
			hook()
		}
	}
	return b.KeyValue.Get(ctx, key)
}

func TestLoginRejectsCredentialChangesDuringVerification(t *testing.T) {
	for _, mutation := range []string{"password", "email", "audit"} {
		t.Run(mutation, func(t *testing.T) {
			var barrier *loginKeyBarrier
			f := newSafetyFixture(t, t.TempDir(), func(keys jetstream.KeyValue) jetstream.KeyValue {
				barrier = &loginKeyBarrier{KeyValue: keys}
				return barrier
			})
			ctx := accountTestContext(t)
			const email = "login@example.invalid"
			const password = "a long login safety password"
			account, err := f.service.CreateLocal(ctx, email, password)
			if err != nil {
				t.Fatal(err)
			}
			other, _, _ := newSafetyReplica(t, f.js, f.stream, f.keys)
			barrier.mu.Lock()
			barrier.hook = func() {
				switch mutation {
				case "password":
					target, err := other.PreparePasswordChange(ctx, account.ID, password, "a different long login password")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := other.ChangePassword(ctx, target); err != nil {
						t.Fatal(err)
					}
				case "email":
					target, err := other.PrepareEmailChange(ctx, account.ID, password, "changed@example.invalid")
					if err != nil {
						t.Fatal(err)
					}
					target, err = other.RecordEmailChangeRequested(ctx, target)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := other.ChangeEmail(ctx, target, "changed@example.invalid"); err != nil {
						t.Fatal(err)
					}
				case "audit":
					if _, ok, err := other.RecordPasswordResetRequested(ctx, email); err != nil || !ok {
						t.Fatalf("audit: ok=%v err=%v", ok, err)
					}
				}
			}
			barrier.mu.Unlock()
			proof, err := f.service.AuthenticateLocal(context.WithValue(ctx, loginReadContextKey{}, true), email, password)
			if mutation == "audit" {
				if err != nil || proof.AuthenticationVersion != account.AuthenticationVersion {
					t.Fatalf("audit staled login: %v", err)
				}
			} else if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("stale login error = %v", err)
			}
		})
	}
}
