package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/storage"
)

func TestWorkflowDeliveryFailureRefundsExistingBudgetKeys(t *testing.T) {
	for _, namespace := range []string{"signup", "password-reset", "email-change"} {
		t.Run(namespace, func(t *testing.T) {
			calls := 0
			sender := &inspectingSender{inspect: func(email.Message) error {
				calls++
				if calls == 1 {
					return errors.New("injected delivery failure")
				}
				return nil
			}}
			runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
			defer stopTestRuntime(t, runtime, cancel, runErrors)
			const address = "delivery-test@example.invalid"
			const password = "a deliberately uncommon delivery test password"
			start := func() (string, error) { return runtime.Registration.Start(t.Context(), address) }
			switch namespace {
			case "password-reset":
				start = func() (string, error) { return runtime.PasswordReset.Start(t.Context(), address) }
			case "email-change":
				account, err := runtime.Accounts.CreateLocal(t.Context(), "old-delivery@example.invalid", password)
				if err != nil {
					t.Fatal(err)
				}
				start = func() (string, error) { return runtime.EmailChange.Start(t.Context(), account.ID, password, address) }
			}
			js, err := jetstream.New(runtime.connection.NATS)
			if err != nil {
				t.Fatal(err)
			}
			stores, err := storage.OpenStores(t.Context(), js, 1)
			if err != nil {
				t.Fatal(err)
			}
			key, err := keyvault.New(stores.Keys).WorkflowKey(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer clear(key)
			digest := hmac.New(sha256.New, key)
			_, _ = digest.Write([]byte("delivery\x00" + address))
			keys := []string{namespace + "-limit.global", namespace + "-limit." + base64.RawURLEncoding.EncodeToString(digest.Sum(nil))}
			if _, err := start(); err == nil || calls != 1 {
				t.Fatalf("failed delivery: calls=%d, error=%v", calls, err)
			}
			for _, key := range keys {
				if _, err := stores.RuntimeState.Get(t.Context(), key); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
					t.Fatalf("failed delivery retained budget: %v", err)
				}
			}
			if _, err := start(); err != nil || calls != 2 {
				t.Fatalf("retry delivery: calls=%d, error=%v", calls, err)
			}
			for _, key := range keys {
				entry, err := stores.RuntimeState.Get(t.Context(), key)
				if err != nil {
					t.Fatal(err)
				}
				if string(entry.Value()) != `{"count":1}` {
					t.Fatal("successful delivery did not preserve the counter format and count")
				}
			}
		})
	}
}
