package jetstreamutil_test

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	. "github.com/chattocorp/chatto/pkg/jetstreamutil"
)

// TestIsSequenceConflictClassifiesClientErrors checks the classification
// against the errors that nats.go returns from a single server. Each write
// gets the server's wrong-last-sequence error (10071). nats.go wraps it in a
// sentinel only for key-value writes. A replicated stream can report a write
// in progress as 10164 instead; TestIsSequenceConflict covers that code.
func TestIsSequenceConflictClassifiesClientErrors(t *testing.T) {
	js, bucket := setupTestKeyValue(t)
	ctx := testContext(t)
	revision, err := bucket.Create(ctx, "key", []byte("first"))
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	stale := revision + 100

	tests := []struct {
		name     string
		write    func() error
		sentinel error
	}{
		{
			name:     "Create of an existing key",
			write:    func() error { _, err := bucket.Create(ctx, "key", []byte("again")); return err },
			sentinel: jetstream.ErrKeyExists,
		},
		{
			name:     "Update with a stale revision",
			write:    func() error { _, err := bucket.Update(ctx, "key", []byte("next"), stale); return err },
			sentinel: jetstream.ErrKeyRevisionMismatch,
		},
		{
			name:     "Delete with a stale revision",
			write:    func() error { return bucket.Delete(ctx, "key", jetstream.LastRevision(stale)) },
			sentinel: jetstream.ErrKeyRevisionMismatch,
		},
		{
			name: "publish with a stale subject sequence",
			write: func() error {
				msg := nats.NewMsg("$KV.KV_TEST.key")
				msg.Data = []byte("raw")
				_, err := js.PublishMsg(ctx, msg, jetstream.WithExpectLastSequencePerSubject(stale))
				return err
			},
		},
		{
			name: "create-once publish of an existing key",
			write: func() error {
				msg := nats.NewMsg("$KV.KV_TEST.key")
				msg.Data = []byte("raw")
				_, err := js.PublishMsg(ctx, msg, jetstream.WithExpectLastSequencePerSubject(0))
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.write()
			if err == nil {
				t.Fatal("write succeeded, want a conflict")
			}
			if !IsSequenceConflict(err) {
				t.Fatalf("IsSequenceConflict(%v) = false, want true", err)
			}
			var apiErr *jetstream.APIError
			if !errors.As(err, &apiErr) || apiErr.ErrorCode != jetstream.JSErrCodeStreamWrongLastSequence {
				t.Fatalf("error %v has no API error code %d", err, jetstream.JSErrCodeStreamWrongLastSequence)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", err, tt.sentinel)
			}
		})
	}
}
