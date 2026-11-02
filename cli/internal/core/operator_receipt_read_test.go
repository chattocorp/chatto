package core

import (
	"context"
	"errors"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Embedding the interface makes any unexpected consumer operation fail the
// test. Receipt reads need only the exact-subject lookup.
type operatorReceiptStream struct {
	jetstream.Stream
	subject string
	data    []byte
	err     error
}

func (s *operatorReceiptStream) GetLastMsgForSubject(_ context.Context, subject string) (*jetstream.RawStreamMsg, error) {
	s.subject = subject
	return &jetstream.RawStreamMsg{Data: s.data}, s.err
}

func TestOperatorReceiptRead(t *testing.T) {
	encode := func(fingerprint, resource string) []byte {
		data, err := proto.Marshal(&evtv1.Event{Event: &evtv1.Event_OperatorOperationCompleted{OperatorOperationCompleted: &evtv1.OperatorOperationCompletedEvent{Fingerprint: fingerprint, ResourceId: resource}}})
		require.NoError(t, err)
		return data
	}
	storageErr := errors.New("storage unavailable")
	for _, tc := range []struct {
		name, resource string
		data           []byte
		readErr        error
		wantErr        bool
	}{
		{name: "absent", readErr: jetstream.ErrMsgNotFound},
		{name: "stored", data: encode("expected", "result"), resource: "result"},
		{name: "different arguments", data: encode("different", "result"), wantErr: true},
		{name: "missing result", data: encode("expected", ""), wantErr: true},
		{name: "wrong event", data: []byte{}, wantErr: true},
		{name: "malformed protobuf", data: []byte{0xff}, wantErr: true},
		{name: "storage failure", readErr: storageErr, wantErr: true},
		{name: "cancelled", readErr: context.Canceled, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := &operatorReceiptStream{data: tc.data, err: tc.readErr}
			op := &operatorOperation{core: &ChattoCore{storage: &storage{serverEvtStream: stream}}, subject: "evt.operator_operation.test.operator_operation_completed", fingerprint: "expected"}
			err := op.load(testContext(t))
			if tc.wantErr {
				require.Error(t, err)
				if tc.readErr != nil {
					require.ErrorIs(t, err, tc.readErr)
				}
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, op.subject, stream.subject)
			require.Equal(t, tc.resource, op.resource)
		})
	}
}

func TestOperatorReceiptExactLookupAndCreationGuard(t *testing.T) {
	t.Parallel()
	c, _ := newTestCore(t)
	ctx := testContext(t)
	info, err := c.storage.serverEvtStream.Info(ctx)
	require.NoError(t, err)
	require.False(t, info.Config.AllowDirect, "receipt decisions must read from the stream leader")
	key := uuid.NewString()
	op, err := c.operatorOperation(ctx, key, "test", "input")
	require.NoError(t, err)
	require.Empty(t, op.resource)
	_, err = c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{op.receipt("result")})
	require.NoError(t, err)
	again, err := c.operatorOperation(ctx, key, "test", "input")
	require.NoError(t, err)
	require.Equal(t, "result", again.resource)
	_, err = c.operatorOperation(ctx, key, "test", "different input")
	require.Error(t, err)
	_, err = c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{op.receipt("other")})
	require.Error(t, err, "a second receipt must never replace the first")
	other, err := c.operatorOperation(ctx, uuid.NewString(), "test", "input")
	require.NoError(t, err)
	require.Empty(t, other.resource, "another operation must not match this receipt")
}
