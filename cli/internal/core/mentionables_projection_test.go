package core

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMentionablesProjection_RetainsOnlyLoginDigestsAndShredReleasesHandle(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	p := NewMentionablesProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "K1",
		Event: &corev1.Event_UserDekGenerated{UserDekGenerated: &corev1.UserDEKGeneratedEvent{
			UserId:        "U1",
			Epoch:         1,
			Purpose:       corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII,
			ContentKeyRef: "dek.test",
		}},
	}, 1))
	contentKey := &messageContentKey{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
	require.NoError(t, p.Apply(userEvent("E1", time.Now(), accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))

	p.RLock()
	require.Equal(t, mentionableLookupKey("Alice"), p.userLogins["U1"])
	_, plaintextOwner := p.owners["alice"]
	p.RUnlock()
	require.False(t, plaintextOwner)
	require.False(t, p.Availability("alice", nil).Available)
	p.dekResolver.keyWrapper = staticProjectionKeyWrapper{unwrapErr: errors.New("KMS unavailable")}
	err = p.Apply(userEvent("E1b", time.Now(), loginChanged(t, contentKey, "E1b", "U1", "Alice2")), 3)
	require.ErrorContains(t, err, "KMS unavailable")
	require.False(t, p.Availability("alice", nil).Available)
	require.True(t, p.Availability("alice2", nil).Available)

	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E2",
		Event: &corev1.Event_UserKeyShredded{UserKeyShredded: &corev1.UserKeyShreddedEvent{
			UserId: "U1",
		}},
	}, 4))
	require.True(t, p.Availability("alice", nil).Available)
	p.RLock()
	_, retained := p.userLogins["U1"]
	p.RUnlock()
	require.False(t, retained)
}
