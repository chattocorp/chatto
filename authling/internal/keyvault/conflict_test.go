package keyvault

import (
	"bytes"
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
)

// raceLoserKeyValue acts like a replica that loses every create race on a
// replicated stream. The first read of each key reports it as missing, so the
// caller tries to create it. Create then fails with the error that nats.go
// returns when it replaces a removal marker and another replica wins. That
// error does not match jetstream.ErrKeyExists. Later reads see the stored key.
type raceLoserKeyValue struct {
	jetstream.KeyValue
	seen map[string]bool
}

func (kv *raceLoserKeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	first := !kv.seen[key]
	kv.seen[key] = true
	if first {
		return nil, jetstream.ErrKeyNotFound
	}
	return kv.KeyValue.Get(ctx, key)
}

func (kv *raceLoserKeyValue) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, &jetstream.APIError{
		Code:        400,
		ErrorCode:   jetstream.JSErrCodeStreamWrongLastSequenceConstant,
		Description: "wrong last sequence",
	}
}

// TestVaultUsesTheWinnerAfterACreateConflict fails when a vault operation
// treats a lost create race as an error instead of using the stored key.
func TestVaultUsesTheWinnerAfterACreateConflict(t *testing.T) {
	signingRef := oidcSigningKeyPrefix + "conflict"
	tests := []struct {
		name string
		// open runs the operation and returns a value that identifies the key.
		// attempt differs between the winner and the loser.
		open func(ctx context.Context, v *Vault, attempt byte) ([]byte, error)
	}{
		{"workflow key", func(ctx context.Context, v *Vault, _ byte) ([]byte, error) {
			return v.WorkflowKey(ctx)
		}},
		{"OIDC token key", func(ctx context.Context, v *Vault, attempt byte) ([]byte, error) {
			return v.OIDCTokenKey(ctx, bytes.Repeat([]byte{attempt}, 32))
		}},
		{"system OIDC signing key", func(ctx context.Context, v *Vault, _ byte) ([]byte, error) {
			key, err := v.OIDCSigningKey(ctx)
			return []byte(key.ID), err
		}},
		{"referenced OIDC signing key", func(ctx context.Context, v *Vault, _ byte) ([]byte, error) {
			key, err := v.EnsureOIDCSigningKey(ctx, signingRef)
			return []byte(key.ID), err
		}},
		{"authentication dummy key", func(ctx context.Context, v *Vault, _ byte) ([]byte, error) {
			_, _, key, err := v.AuthenticationDummyKey(ctx)
			return key, err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keys := openTestKeys(t)
			winner, err := test.open(t.Context(), New(keys), 1)
			if err != nil {
				t.Fatal(err)
			}
			loser := New(&raceLoserKeyValue{KeyValue: keys, seen: make(map[string]bool)})
			got, err := test.open(t.Context(), loser, 2)
			if err != nil {
				t.Fatalf("operation after a lost create race: %v", err)
			}
			if !bytes.Equal(got, winner) {
				t.Fatal("operation did not return the stored key")
			}
		})
	}
}

func openTestKeys(t *testing.T) jetstream.KeyValue {
	t.Helper()
	connection, err := natsruntime.Open(t.Context(), config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	js, _, err := storage.Open(t.Context(), connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	return stores.Keys
}
