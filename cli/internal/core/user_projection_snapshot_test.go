package core

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestUserProjectionSnapshotRoundTripExcludesAuthenticationState(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	newProjection := func() *UserProjection {
		return NewUserProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	}
	original := newProjection()
	contentKey := &messageContentKey{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
	createdAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)

	eventsToApply := []*corev1.Event{
		{Id: "K1", Event: &corev1.Event_UserDekGenerated{UserDekGenerated: &corev1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"}}},
		userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "alice-private", "Alice Private")),
		{Id: "E2", Event: &corev1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("password-hash-secret")}}},
		{Id: "E3", Event: &corev1.Event_UserExternalIdentityLinked{UserExternalIdentityLinked: &corev1.UserExternalIdentityLinkedEvent{UserId: "U1", Issuer: "https://private-issuer.example", Subject: "private-provider-subject", ProviderId: "private-provider"}}},
		{Id: "E4", Event: &corev1.Event_OauthConsentGranted{OauthConsentGranted: &corev1.OAuthConsentGrantedEvent{UserId: "U1", RedirectOrigin: "https://private-client.example"}}},
		userEvent("E5", createdAt.Add(time.Minute), &corev1.Event{Event: &corev1.Event_UserServerPreferencesChanged{UserServerPreferencesChanged: &corev1.UserServerPreferencesChangedEvent{UserId: "U1", Preferences: &corev1.ServerUserPreferences{Timezone: proto.String("Europe/Berlin")}}}}),
	}
	for i, event := range eventsToApply {
		require.NoError(t, original.Apply(event, uint64(i+1)))
	}

	payload, err := original.Snapshot()
	require.NoError(t, err)
	require.NotEmpty(t, payload)
	for _, secret := range [][]byte{
		[]byte("alice-private"), []byte("Alice Private"), []byte("password-hash-secret"),
		[]byte("private-issuer"), []byte("private-provider-subject"), []byte("private-provider"), []byte("private-client"),
	} {
		require.Falsef(t, bytes.Contains(payload, secret), "snapshot contains forbidden value %q", secret)
	}

	restored := newProjection()
	require.NoError(t, restored.Restore(payload))
	user, ok := restored.Get("U1")
	require.True(t, ok)
	require.Equal(t, "alice-private", user.GetLogin())
	require.Equal(t, "Alice Private", user.GetDisplayName())
	preferences, ok := restored.Preferences("U1")
	require.True(t, ok)
	require.Equal(t, "Europe/Berlin", preferences.GetTimezone())
	_, ok = restored.PasswordHash("U1")
	require.False(t, ok, "password credentials must not be restored from a profile snapshot")
	require.Empty(t, restored.ExternalIdentities("U1"), "external identities must not be restored from a profile snapshot")
	require.False(t, restored.HasOAuthConsent("U1", "https://private-client.example"), "OAuth consent must not be restored from a profile snapshot")
}

func TestUserProjectionSnapshotIsDeterministicAndTailReplayMatchesColdReplay(t *testing.T) {
	original, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	created := userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A."))
	require.NoError(t, original.Apply(created, 2))
	first, err := original.Snapshot()
	require.NoError(t, err)
	second, err := original.Snapshot()
	require.NoError(t, err)
	require.Equal(t, first, second)

	restored := NewUserProjection(staticProjectionKeyWrapper{key: contentKey.key}, staticProjectionDEKStore{})
	require.NoError(t, restored.Restore(first))
	tail := userEvent("E2", createdAt.Add(time.Minute), loginChanged(t, contentKey, "E2", "U1", "Alice2"))
	require.NoError(t, restored.Apply(tail, 3))

	cold := NewUserProjection(staticProjectionKeyWrapper{key: contentKey.key}, staticProjectionDEKStore{})
	require.NoError(t, cold.Apply(&corev1.Event{Id: "K1", Event: &corev1.Event_UserDekGenerated{UserDekGenerated: &corev1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"}}}, 1))
	require.NoError(t, cold.Apply(created, 2))
	require.NoError(t, cold.Apply(tail, 3))
	require.Equal(t, cold.Users(), restored.Users())
}

func TestUserProjectionRestoreIsTransactionalAndDoesNotTouchAuthState(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	require.NoError(t, p.Apply(userEvent("E1", time.Now(), accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice")), 2))
	require.NoError(t, p.Apply(&corev1.Event{Id: "E2", Event: &corev1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("hash")}}}, 3))

	require.Error(t, p.Restore([]byte{0xff}))
	_, ok := p.Get("U1")
	require.True(t, ok, "failed restore must preserve profile state")
	hash, ok := p.PasswordHash("U1")
	require.True(t, ok, "failed restore must preserve auth state")
	require.Equal(t, []byte("hash"), hash)

	require.NoError(t, p.Restore(nil))
	_, ok = p.Get("U1")
	require.False(t, ok, "empty restore must reset profile state")
	hash, ok = p.PasswordHash("U1")
	require.True(t, ok, "profile restore must never reset independently replayed auth state")
	require.Equal(t, []byte("hash"), hash)
}

func TestUserProjectionRestoreRejectsPlaintextUserFields(t *testing.T) {
	payload, err := proto.Marshal(&corev1.UserProfileProjectionSnapshot{Users: []*corev1.ProjectedUserProfileSnapshot{{
		UserId: "U1", User: &corev1.User{Id: "U1", Login: "plaintext"},
	}}})
	require.NoError(t, err)
	p := NewUserProjection(nil, nil)
	require.ErrorContains(t, p.Restore(payload), "plaintext user")
}

func TestUserAuthProjectionSubjectsStayFocused(t *testing.T) {
	p := newUserAuthProjection()
	require.NotContains(t, p.Subjects(), events.UserSubjectFilter())
	require.Len(t, p.Subjects(), 8)
}

func TestUserAuthProjectionRebuildsAndRevokesCredentialState(t *testing.T) {
	p := newUserAuthProjection()
	createdAt := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	eventsToApply := []*corev1.Event{
		userEvent("A1", createdAt, &corev1.Event{Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U1"}}}),
		userEvent("A2", createdAt.Add(time.Minute), &corev1.Event{Event: &corev1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("hash")}}}),
		{Id: "A3", Event: &corev1.Event_UserExternalIdentityLinked{UserExternalIdentityLinked: &corev1.UserExternalIdentityLinkedEvent{UserId: "U1", Issuer: "issuer", Subject: "subject", ProviderId: "provider"}}},
		{Id: "A4", Event: &corev1.Event_OauthConsentGranted{OauthConsentGranted: &corev1.OAuthConsentGrantedEvent{UserId: "U1", RedirectOrigin: "https://client.example"}}},
	}
	for i, event := range eventsToApply {
		require.NoError(t, p.Apply(event, uint64(i+1)))
	}
	hash, setAt, ok := p.PasswordHashWithSetAt("U1")
	require.True(t, ok)
	require.Equal(t, []byte("hash"), hash)
	require.Equal(t, createdAt.Add(time.Minute), setAt)
	require.Equal(t, uint64(2), mustAuthGeneration(t, p, "U1"))
	owner, ok := p.ExternalIdentityOwnerID("issuer", "subject")
	require.True(t, ok)
	require.Equal(t, "U1", owner)
	require.True(t, p.HasOAuthConsent("U1", "https://client.example"))

	require.NoError(t, p.Apply(&corev1.Event{Id: "A5", Event: &corev1.Event_UserAccountDeleted{UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: "U1"}}}, 5))
	_, _, ok = p.PasswordHashWithSetAt("U1")
	require.False(t, ok)
	_, ok = p.ExternalIdentityOwnerID("issuer", "subject")
	require.False(t, ok)
	require.False(t, p.HasOAuthConsent("U1", "https://client.example"))
	_, ok = p.AuthGeneration("U1")
	require.False(t, ok)
}

func mustAuthGeneration(t *testing.T, p *UserAuthProjection, userID string) uint64 {
	t.Helper()
	generation, ok := p.AuthGeneration(userID)
	require.True(t, ok)
	return generation
}
