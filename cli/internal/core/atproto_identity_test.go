package core

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

func TestValidateAtprotoDID(t *testing.T) {
	tests := []struct {
		did   string
		valid bool
	}{
		{"did:plc:z72i7hdynmk6r22z27h6tvur", true},
		{"did:web:example.com", true},
		{"did:web:localhost%3A8080", true},
		{"did:key:z6Mkabc", false},
		{"did:plc:", false},
		{"did:plc:abc:", false},
		{"alice.bsky.social", false},
		{"did:plc:abc def", false},
		{"did:web:" + strings.Repeat("a", MaxAtprotoDIDLength), false},
	}
	for _, tt := range tests {
		t.Run(tt.did[:min(len(tt.did), 40)], func(t *testing.T) {
			err := ValidateAtprotoDID(tt.did)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInvalidArgument)
			}
		})
	}
}

func TestChattoCore_SetAndClearUserAtprotoDID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "atprotouser", "AT User", "password123")
	require.NoError(t, err)
	didEvents := func() int {
		events, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserAtprotoDIDChanged))
		require.NoError(t, err)
		return len(events)
	}

	// Clearing without a DID is a no-op.
	require.NoError(t, core.ClearUserAtprotoDID(ctx, user.Id))
	require.Equal(t, 0, didEvents())

	_, err = core.SetUserAtprotoDID(ctx, user.Id, "alice.bsky.social")
	require.True(t, errors.Is(err, ErrInvalidArgument), "handle must be rejected, got %v", err)

	did := "did:plc:z72i7hdynmk6r22z27h6tvur"
	got, err := core.SetUserAtprotoDID(ctx, user.Id, did)
	require.NoError(t, err)
	require.Equal(t, did, got)
	projected, err := core.GetUser(ctx, user.Id)
	require.NoError(t, err)
	require.Equal(t, did, projected.GetAtprotoDID())
	require.Equal(t, 1, didEvents())

	// The DID is PII: the stored fact contains only ciphertext.
	stored, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserAtprotoDIDChanged))
	require.NoError(t, err)
	require.NotNil(t, stored[0].GetUserAtprotoDidChanged().GetEncryptedDid())

	// Setting the same DID appends nothing.
	_, err = core.SetUserAtprotoDID(ctx, user.Id, did)
	require.NoError(t, err)
	require.Equal(t, 1, didEvents())

	require.NoError(t, core.ClearUserAtprotoDID(ctx, user.Id))
	projected, err = core.GetUser(ctx, user.Id)
	require.NoError(t, err)
	require.Empty(t, projected.GetAtprotoDID())
	require.Equal(t, 2, didEvents())
}

func TestUserProjectionSnapshotPreservesEncryptedAtprotoDID(t *testing.T) {
	original, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	require.NoError(t, original.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "alice", "Alice")), 2))
	did := "did:plc:snapshotdidvalue"
	encrypted, err := encryptUserPIIStringWithContentKey(contentKey, "E2", "U1", evtstream.EventUserAtprotoDIDChanged, "atproto_did", did)
	require.NoError(t, err)
	require.NoError(t, original.Apply(userEvent("E2", createdAt.Add(time.Minute), &evtv1.Event{
		Event: &evtv1.Event_UserAtprotoDidChanged{UserAtprotoDidChanged: &evtv1.UserAtprotoDidChangedEvent{UserId: "U1", EncryptedDid: encrypted}},
	}), 3))

	payload, err := original.Snapshot()
	require.NoError(t, err)
	require.False(t, bytes.Contains(payload, []byte(did)), "snapshot must not contain the plaintext DID")

	restored := NewUserProjection(staticProjectionKeyWrapper{key: contentKey.key}, staticProjectionDEKStore{})
	require.NoError(t, restored.Restore(payload))
	user, ok := restored.Get("U1")
	require.True(t, ok)
	require.Equal(t, did, user.GetAtprotoDID())
}
