package core

import (
	"bytes"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestUserProjectionSnapshotRoundTripExcludesAuthenticationState(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	newProjection := func() *UserProjection {
		return NewUserProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	}
	original := newProjection()
	contentKey := &messageContentKey{epoch: 1, purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
	createdAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)

	eventsToApply := []*evtv1.Event{
		{Id: "K1", Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"}}},
		userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "alice-private", "Alice Private")),
		{Id: "E2", Event: &evtv1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("password-hash-secret")}}},
		{Id: "E3", Event: &evtv1.Event_UserExternalIdentityLinked{UserExternalIdentityLinked: &evtv1.UserExternalIdentityLinkedEvent{UserId: "U1", Issuer: "https://private-issuer.example", Subject: "private-provider-subject", ProviderId: "private-provider"}}},
		{Id: "E4", Event: &evtv1.Event_OauthConsentGranted{OauthConsentGranted: &evtv1.OAuthConsentGrantedEvent{UserId: "U1", RedirectOrigin: "https://private-client.example"}}},
		userEvent("E5", createdAt.Add(time.Minute), &evtv1.Event{Event: &evtv1.Event_UserServerPreferencesChanged{UserServerPreferencesChanged: &evtv1.UserServerPreferencesChangedEvent{UserId: "U1", Preferences: &evtv1.ServerUserPreferences{Timezone: proto.String("Europe/Berlin")}}}}),
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

func TestUserProjectionSnapshotPreservesBotIdentityAndOwnerIndex(t *testing.T) {
	original, contentKey := newEncryptedUserProjection(t, "owner")
	createdAt := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	require.NoError(t, original.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "owner", "owner", "Owner")), 2))
	require.NoError(t, original.Apply(&evtv1.Event{
		Id: "K2",
		Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
			UserId: "bot", Epoch: 1, Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test",
		}},
	}, 3))
	botCreated := accountCreated(t, contentKey, "E2", "bot", "helper_bot", "Helper Bot")
	botCreated.GetUserAccountCreated().IsBot = true
	botCreated.GetUserAccountCreated().BotOwnerUserId = "owner"
	require.NoError(t, original.Apply(userEvent("E2", createdAt.Add(time.Minute), botCreated), 4))
	require.NoError(t, original.Apply(&evtv1.Event{
		Id: "E3",
		Event: &evtv1.Event_BotOwnerReassigned{BotOwnerReassigned: &evtv1.BotOwnerReassignedEvent{
			UserId: "bot", PreviousOwnerUserId: "owner", OwnerUserId: "new-owner",
		}},
	}, 5))

	payload, err := original.Snapshot()
	require.NoError(t, err)
	restored := NewUserProjection(staticProjectionKeyWrapper{key: contentKey.key}, staticProjectionDEKStore{})
	require.NoError(t, restored.Restore(payload))

	bot, ok := restored.Get("bot")
	require.True(t, ok)
	require.True(t, bot.GetIsBot())
	require.Equal(t, "new-owner", bot.GetBotOwnerUserId())
	require.Equal(t, []string{"bot"}, restored.BotIDs())
	require.Empty(t, restored.BotIDsOwnedBy("owner"))
	require.Equal(t, []string{"bot"}, restored.BotIDsOwnedBy("new-owner"))
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
	require.NoError(t, cold.Apply(&evtv1.Event{Id: "K1", Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"}}}, 1))
	require.NoError(t, cold.Apply(created, 2))
	require.NoError(t, cold.Apply(tail, 3))
	require.Equal(t, cold.Users(), restored.Users())
}

func TestUserProjectionSnapshotPreservesCanonicalOwnersForDuplicateDigests(t *testing.T) {
	original, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	require.NoError(t, original.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice One")), 2))
	require.NoError(t, original.Apply(&evtv1.Event{
		Id: "K2",
		Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
			UserId: "U2", Epoch: 1, Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test",
		}},
	}, 3))
	require.NoError(t, original.Apply(userEvent("E2", createdAt.Add(time.Minute), accountCreated(t, contentKey, "E2", "U2", "Alice", "Alice Two")), 4))
	for seq, userID := range []string{"U1", "U2"} {
		eventID := fmt.Sprintf("M%d", seq+1)
		encryptedEmail, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, evtstream.EventUserVerifiedEmailAdded, "email", "alice@example.com")
		require.NoError(t, err)
		require.NoError(t, original.Apply(&evtv1.Event{
			Id: eventID,
			Event: &evtv1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &evtv1.UserVerifiedEmailAddedEvent{
				UserId: userID, EncryptedEmail: encryptedEmail,
			}},
		}, uint64(seq+5)))
	}

	payload, err := original.Snapshot()
	require.NoError(t, err)
	restored := NewUserProjection(staticProjectionKeyWrapper{key: contentKey.key}, staticProjectionDEKStore{})
	require.NoError(t, restored.Restore(payload))

	byLogin, ok := restored.GetByLogin("alice")
	require.True(t, ok)
	require.Equal(t, "U2", byLogin.GetId(), "the last login event remains the canonical owner")
	byEmail, ok := restored.GetByEmail("Alice@Example.com")
	require.True(t, ok)
	require.Equal(t, "U2", byEmail.GetId(), "the last verified-email event remains the canonical owner")
	require.Len(t, restored.Users(), 2, "both historical profile rows remain available")
}

func TestUserProjectionSnapshotPreservesUnclaimedDuplicateDigests(t *testing.T) {
	original, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	require.NoError(t, original.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice One")), 2))
	require.NoError(t, original.Apply(&evtv1.Event{
		Id: "K2",
		Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
			UserId: "U2", Epoch: 1, Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test",
		}},
	}, 3))
	require.NoError(t, original.Apply(userEvent("E2", createdAt.Add(time.Minute), accountCreated(t, contentKey, "E2", "U2", "Alice", "Alice Two")), 4))
	for seq, userID := range []string{"U1", "U2"} {
		eventID := fmt.Sprintf("M%d", seq+1)
		encryptedEmail, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, evtstream.EventUserVerifiedEmailAdded, "email", "alice@example.com")
		require.NoError(t, err)
		require.NoError(t, original.Apply(&evtv1.Event{
			Id: eventID,
			Event: &evtv1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &evtv1.UserVerifiedEmailAddedEvent{
				UserId: userID, EncryptedEmail: encryptedEmail,
			}},
		}, uint64(seq+5)))
	}
	require.NoError(t, original.Apply(userEvent("E3", createdAt.Add(2*time.Minute), loginChanged(t, contentKey, "E3", "U2", "Bob")), 7))
	require.NoError(t, original.Apply(&evtv1.Event{
		Id: "E4", Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: "U2"}},
	}, 8))

	payload, err := original.Snapshot()
	require.NoError(t, err)
	restored := NewUserProjection(staticProjectionKeyWrapper{key: contentKey.key}, staticProjectionDEKStore{})
	require.NoError(t, restored.Restore(payload))

	_, ok := restored.GetByLogin("Alice")
	require.False(t, ok, "an older duplicate login must not regain ownership")
	_, ok = restored.GetByLogin("Bob")
	require.False(t, ok, "a deleted owner's login must remain unclaimed")
	_, ok = restored.GetByEmail("alice@example.com")
	require.False(t, ok, "an older duplicate email must not regain ownership")
	_, ok = restored.Get("U1")
	require.True(t, ok, "the older active profile remains available by ID")
}

func TestUserProjectionRestoreIsTransactionalAndDoesNotTouchAuthState(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	require.NoError(t, p.Apply(userEvent("E1", time.Now(), accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice")), 2))
	require.NoError(t, p.Apply(&evtv1.Event{Id: "E2", Event: &evtv1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("hash")}}}, 3))

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
	payload, err := proto.Marshal(&projectionv1.UserProfileProjectionSnapshot{Users: []*projectionv1.ProjectedUserProfileSnapshot{{
		UserId: "U1", User: &evtv1.User{Id: "U1", Login: "plaintext"},
	}}})
	require.NoError(t, err)
	p := NewUserProjection(nil, nil)
	require.ErrorContains(t, p.Restore(payload), "plaintext user")
}

func TestUserProjectionRestoreRejectsInconsistentProfileState(t *testing.T) {
	pii := func(purpose string) *projectionv1.ProjectedEncryptedUserStringSnapshot {
		return &projectionv1.ProjectedEncryptedUserStringSnapshot{
			EventId: "E1", EventType: evtstream.EventUserAccountCreated, Purpose: purpose,
			Encrypted: &evtv1.EncryptedUserString{EncryptedValue: []byte("ciphertext"), Nonce: []byte("nonce"), ContentKeyEpoch: 1},
		}
	}
	valid := &projectionv1.UserProfileProjectionSnapshot{
		Keys: []*evtv1.UserDEKGeneratedEvent{{UserId: "U1", Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, Epoch: 1, ContentKeyRef: "dek.test"}},
		Users: []*projectionv1.ProjectedUserProfileSnapshot{{
			UserId: "U1", User: &evtv1.User{Id: "U1"}, Login: pii("login"), LoginHash: "digest", DisplayName: pii("display_name"),
		}},
		LoginIndex: []*projectionv1.StringStringSnapshot{{Key: "digest", Value: "U1"}},
	}
	tests := []struct {
		name   string
		mutate func(*projectionv1.UserProfileProjectionSnapshot)
	}{
		{"missing user", func(snapshot *projectionv1.UserProfileProjectionSnapshot) { snapshot.Users[0].User = nil }},
		{"missing display name", func(snapshot *projectionv1.UserProfileProjectionSnapshot) { snapshot.Users[0].DisplayName = nil }},
		{"missing profile DEK", func(snapshot *projectionv1.UserProfileProjectionSnapshot) { snapshot.Keys = nil }},
		{"inactive user retains profile", func(snapshot *projectionv1.UserProfileProjectionSnapshot) { snapshot.Users[0].Deleted = true }},
		{"unknown login owner", func(snapshot *projectionv1.UserProfileProjectionSnapshot) { snapshot.LoginIndex[0].Value = "U2" }},
		{"mismatched login digest", func(snapshot *projectionv1.UserProfileProjectionSnapshot) { snapshot.LoginIndex[0].Key = "other" }},
		{"duplicate login digest", func(snapshot *projectionv1.UserProfileProjectionSnapshot) {
			snapshot.LoginIndex = append(snapshot.LoginIndex, proto.Clone(snapshot.LoginIndex[0]).(*projectionv1.StringStringSnapshot))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := proto.Clone(valid).(*projectionv1.UserProfileProjectionSnapshot)
			tt.mutate(snapshot)
			payload, err := proto.Marshal(snapshot)
			require.NoError(t, err)
			require.Error(t, NewUserProjection(nil, nil).Restore(payload))
		})
	}
}

func TestUserAuthProjectionSubjectsStayFocused(t *testing.T) {
	p := newUserAuthProjection()
	require.NotContains(t, p.Subjects(), evtstream.UserSubjectFilter())
	require.Len(t, p.Subjects(), 18)
}

func TestUserAuthProjectionReplaysBotIncomingWebhookLifecycle(t *testing.T) {
	p := newUserAuthProjection()
	createdAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	require.NoError(t, p.Apply(userEvent("W1", createdAt, &evtv1.Event{Event: &evtv1.Event_UserAccountCreated{
		UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "U-bot", IsBot: true, BotOwnerUserId: "U-owner"},
	}}), 1))
	require.NoError(t, p.Apply(userEvent("W2", createdAt.Add(time.Minute), &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookCreated{
		BotIncomingWebhookCreated: &evtv1.BotIncomingWebhookCreatedEvent{UserId: "U-bot", Verifier: []byte("first")},
	}}), 2))
	credential, ok := p.BotIncomingWebhookCredential("U-bot", legacyBotIncomingWebhookID)
	require.True(t, ok)
	require.Equal(t, []byte("first"), credential.Verifier)
	require.Equal(t, createdAt.Add(time.Minute), credential.CreatedAt)

	require.NoError(t, p.Apply(userEvent("W3", createdAt.Add(2*time.Minute), &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookRotated{
		BotIncomingWebhookRotated: &evtv1.BotIncomingWebhookRotatedEvent{UserId: "U-bot", Verifier: []byte("second")},
	}}), 3))
	credential, ok = p.BotIncomingWebhookCredential("U-bot", legacyBotIncomingWebhookID)
	require.True(t, ok)
	require.Equal(t, []byte("second"), credential.Verifier)

	require.NoError(t, p.Apply(userEvent("W4", createdAt.Add(3*time.Minute), &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookRevoked{
		BotIncomingWebhookRevoked: &evtv1.BotIncomingWebhookRevokedEvent{UserId: "U-bot"},
	}}), 4))
	_, ok = p.BotIncomingWebhookCredential("U-bot", legacyBotIncomingWebhookID)
	require.False(t, ok)
}

func TestUserAuthProjectionRebuildsAndRevokesCredentialState(t *testing.T) {
	p := newUserAuthProjection()
	createdAt := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	eventsToApply := []*evtv1.Event{
		userEvent("A1", createdAt, &evtv1.Event{Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "U1"}}}),
		userEvent("A2", createdAt.Add(time.Minute), &evtv1.Event{Event: &evtv1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("hash")}}}),
		{Id: "A3", Event: &evtv1.Event_UserExternalIdentityLinked{UserExternalIdentityLinked: &evtv1.UserExternalIdentityLinkedEvent{UserId: "U1", Issuer: "issuer", Subject: "subject", ProviderId: "provider"}}},
		{Id: "A4", Event: &evtv1.Event_OauthConsentGranted{OauthConsentGranted: &evtv1.OAuthConsentGrantedEvent{UserId: "U1", RedirectOrigin: "https://client.example"}}},
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

	require.NoError(t, p.Apply(&evtv1.Event{Id: "A5", Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: "U1"}}}, 5))
	_, _, ok = p.PasswordHashWithSetAt("U1")
	require.False(t, ok)
	_, ok = p.ExternalIdentityOwnerID("issuer", "subject")
	require.False(t, ok)
	require.False(t, p.HasOAuthConsent("U1", "https://client.example"))
	_, ok = p.AuthGeneration("U1")
	require.False(t, ok)
}

func TestUserAuthProjectionShreddingRequestIsTerminal(t *testing.T) {
	p := newUserAuthProjection()
	require.NoError(t, p.Apply(&evtv1.Event{Id: "A1", Event: &evtv1.Event_UserAccountCreated{
		UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "U1"},
	}}, 1))
	require.NoError(t, p.Apply(&evtv1.Event{Id: "A2", Event: &evtv1.Event_UserPasswordHashChanged{
		UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("before")},
	}}, 2))
	require.NoError(t, p.Apply(&evtv1.Event{Id: "A3", Event: &evtv1.Event_UserKeyShreddingRequested{
		UserKeyShreddingRequested: &evtv1.UserKeyShreddingRequestedEvent{UserId: "U1"},
	}}, 3))
	require.NoError(t, p.Apply(&evtv1.Event{Id: "A4", Event: &evtv1.Event_UserPasswordHashChanged{
		UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{UserId: "U1", PasswordHash: []byte("late")},
	}}, 4))
	_, _, ok := p.PasswordHashWithSetAt("U1")
	require.False(t, ok)
}

func mustAuthGeneration(t *testing.T, p *UserAuthProjection, userID string) uint64 {
	t.Helper()
	generation, ok := p.AuthGeneration(userID)
	require.True(t, ok)
	return generation
}
