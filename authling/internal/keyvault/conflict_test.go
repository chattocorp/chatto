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

// tombstoneConflictKeyValue rejects Create with the error that nats.go returns
// when it replaces a removal marker and another replica wins the race. That
// error does not match jetstream.ErrKeyExists.
type tombstoneConflictKeyValue struct{ jetstream.KeyValue }

func (kv tombstoneConflictKeyValue) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, &jetstream.APIError{
		Code:        400,
		ErrorCode:   jetstream.JSErrCodeStreamWrongLastSequenceConstant,
		Description: "wrong last sequence",
	}
}

func TestOIDCTokenKeyUsesTheWinnerAfterAConflict(t *testing.T) {
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
	winner := bytes.Repeat([]byte{7}, 32)
	if _, err := New(stores.Keys).OIDCTokenKey(t.Context(), winner); err != nil {
		t.Fatal(err)
	}

	// A replica that did not see the winner's key tries to create its own.
	loser := &Vault{kv: tombstoneConflictKeyValue{&missingThenStoredKeyValue{KeyValue: stores.Keys}}}
	got, err := loser.OIDCTokenKey(t.Context(), bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatalf("OIDCTokenKey after a conflict: %v", err)
	}
	if !bytes.Equal(got, winner) {
		t.Fatal("OIDCTokenKey did not return the stored key")
	}
}

// missingThenStoredKeyValue reports the first read as missing, so the caller
// tries to create the key. Later reads see the stored key.
type missingThenStoredKeyValue struct {
	jetstream.KeyValue
	reads int
}

func (kv *missingThenStoredKeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.reads++
	if kv.reads == 1 {
		return nil, jetstream.ErrKeyNotFound
	}
	return kv.KeyValue.Get(ctx, key)
}
