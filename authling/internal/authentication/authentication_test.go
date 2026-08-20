package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/datacrypto"
)

type stubAuthenticator struct{ err error }

func (s *stubAuthenticator) AuthenticateLocal(context.Context, string, string) (accounts.Account, error) {
	return accounts.Account{}, s.err
}

func (s *stubAuthenticator) PrepareEmailChange(context.Context, string, string, string) (accounts.EmailChangeTarget, error) {
	return accounts.EmailChangeTarget{}, s.err
}

func TestOperationalFailureDoesNotConsumeAttemptBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}
	key, err := datacrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	authenticator := &stubAuthenticator{err: errors.New("key service unavailable")}
	service := New(stores.RuntimeState, js, key, authenticator)
	clear(key)

	if _, err := service.Login(ctx, "person@example.com", "password"); err == nil || errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("operational login error = %v", err)
	}
	keyName := service.attemptKey("login-attempt", "person@example.com")
	if _, err := stores.RuntimeState.Get(ctx, keyName); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("attempt counter after operational error = %v, want absent", err)
	}

	authenticator.err = accounts.ErrInvalidCredentials
	if _, err := service.Login(ctx, "person@example.com", "password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("credential mismatch error = %v", err)
	}
	entry, err := stores.RuntimeState.Get(ctx, keyName)
	if err != nil {
		t.Fatal(err)
	}
	var counter attemptCounter
	if err := json.Unmarshal(entry.Value(), &counter); err != nil || counter.Count != 1 {
		t.Fatalf("attempt counter = %+v, decode error = %v; want count 1", counter, err)
	}
}
