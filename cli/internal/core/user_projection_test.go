package core

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func BenchmarkUserProjectionGetReferences(b *testing.B) {
	const memberCount = 10_000
	key, err := encryption.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	p := NewUserProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	userIDs := make([]string, memberCount)
	for i := range memberCount {
		userID := fmt.Sprintf("user-%05d", i)
		eventID := fmt.Sprintf("event-%05d", i)
		userIDs[i] = userID
		contentKey := &messageContentKey{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
		encryptedLogin, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, events.EventUserAccountCreated, "login", userID)
		if err != nil {
			b.Fatal(err)
		}
		encryptedDisplayName, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, events.EventUserAccountCreated, "display_name", userID)
		if err != nil {
			b.Fatal(err)
		}
		p.users[userID] = &projectedUser{
			user:        &corev1.User{Id: userID},
			login:       newProjectedUserPII(eventID, events.EventUserAccountCreated, "login", encryptedLogin),
			displayName: newProjectedUserPII(eventID, events.EventUserAccountCreated, "display_name", encryptedDisplayName),
		}
		p.dekEvents[userID] = map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent{
			corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII: {
				1: {UserId: userID, Epoch: 1, Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"},
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		if got := p.GetReferences(userIDs); len(got) != memberCount {
			b.Fatalf("GetReferences() returned %d users, want %d", len(got), memberCount)
		}
	}
}

func userEvent(id string, ts time.Time, event *corev1.Event) *corev1.Event {
	event.Id = id
	event.CreatedAt = timestamppb.New(ts)
	return event
}

type staticProjectionKeyWrapper struct {
	key         []byte
	unwrapCalls *int
	unwrapErr   error
}

type staticProjectionDEKStore struct{}

func (w staticProjectionKeyWrapper) CreateKey(context.Context, string) (string, error) {
	return "test-key", nil
}

func (w staticProjectionKeyWrapper) KeyExists(context.Context, string) (bool, error) {
	return true, nil
}

func (w staticProjectionKeyWrapper) WrapContentKey(context.Context, string, []byte, []byte) (*kms.WrappedContentKey, error) {
	return nil, nil
}

func (w staticProjectionKeyWrapper) UnwrapContentKey(context.Context, string, kms.WrappedContentKey, []byte) ([]byte, error) {
	if w.unwrapCalls != nil {
		(*w.unwrapCalls)++
	}
	if w.unwrapErr != nil {
		return nil, w.unwrapErr
	}
	return append([]byte(nil), w.key...), nil
}

func (w staticProjectionKeyWrapper) ShredKey(context.Context, string) error {
	return nil
}

func (s staticProjectionDEKStore) Get(context.Context, string) (*corev1.UserDataEncryptionKey, error) {
	return &corev1.UserDataEncryptionKey{
		EncryptedContentKey: []byte("wrapped"),
		ContentKeyNonce:     []byte("nonce"),
		WrappingKeyRef:      "test-key",
	}, nil
}

func newEncryptedUserProjection(t *testing.T, userID string) (*UserProjection, *messageContentKey) {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	p := NewUserProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "K1",
		Event: &corev1.Event_UserDekGenerated{UserDekGenerated: &corev1.UserDEKGeneratedEvent{
			UserId:        userID,
			Epoch:         1,
			Purpose:       corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII,
			ContentKeyRef: "dek.test",
		}},
	}, 1))
	return p, &messageContentKey{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
}

func accountCreated(t *testing.T, contentKey *messageContentKey, eventID, userID, login, displayName string) *corev1.Event {
	t.Helper()
	encryptedLogin, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, events.EventUserAccountCreated, "login", login)
	require.NoError(t, err)
	encryptedDisplayName, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, events.EventUserAccountCreated, "display_name", displayName)
	require.NoError(t, err)
	return &corev1.Event{Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{
		UserId:               userID,
		EncryptedLogin:       encryptedLogin,
		EncryptedDisplayName: encryptedDisplayName,
	}}}
}

func loginChanged(t *testing.T, contentKey *messageContentKey, eventID, userID, login string) *corev1.Event {
	t.Helper()
	encryptedLogin, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, events.EventUserLoginChanged, "login", login)
	require.NoError(t, err)
	return &corev1.Event{Event: &corev1.Event_UserLoginChanged{UserLoginChanged: &corev1.UserLoginChangedEvent{
		UserId:         userID,
		EncryptedLogin: encryptedLogin,
	}}}
}

func loginCooldownStarted(userID string) *corev1.Event {
	return &corev1.Event{Event: &corev1.Event_UserLoginCooldownStarted{UserLoginCooldownStarted: &corev1.UserLoginCooldownStartedEvent{
		UserId: userID,
	}}}
}

func displayNameChanged(t *testing.T, contentKey *messageContentKey, eventID, userID, displayName string) *corev1.Event {
	t.Helper()
	encryptedDisplayName, err := encryptUserPIIStringWithContentKey(contentKey, eventID, userID, events.EventUserDisplayNameChanged, "display_name", displayName)
	require.NoError(t, err)
	return &corev1.Event{Event: &corev1.Event_UserDisplayNameChanged{UserDisplayNameChanged: &corev1.UserDisplayNameChangedEvent{
		UserId:               userID,
		EncryptedDisplayName: encryptedDisplayName,
	}}}
}

func TestUserProjection_AccountProfileAndLogin(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))

	got, ok := p.Get("U1")
	require.True(t, ok)
	require.Equal(t, "U1", got.GetId())
	require.Equal(t, "Alice", got.GetLogin())
	require.Equal(t, "Alice A.", got.GetDisplayName())
	require.True(t, got.GetCreatedAt().AsTime().Equal(createdAt))

	byLogin, ok := p.GetByLogin("alice")
	require.True(t, ok)
	require.Equal(t, "U1", byLogin.GetId())
	require.Equal(t, 1, p.Count())
}

func TestUserProjection_RetainsEncryptedPIIAndDecryptsOnRead(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	unwrapCalls := 0
	p := NewUserProjection(staticProjectionKeyWrapper{key: key, unwrapCalls: &unwrapCalls}, staticProjectionDEKStore{})
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
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))
	require.Equal(t, 1, unwrapCalls, "projection apply decrypts login transiently to derive its lookup digest")

	p.RLock()
	projected := p.users["U1"]
	require.NotNil(t, projected)
	require.Empty(t, projected.user.GetLogin())
	require.Empty(t, projected.user.GetDisplayName())
	require.NotEmpty(t, projected.login.encrypted.GetEncryptedValue())
	require.NotEmpty(t, projected.displayName.encrypted.GetEncryptedValue())
	require.Equal(t, "U1", p.loginIndex[userPIILookupHash("Alice")])
	_, plaintextIndexed := p.loginIndex["alice"]
	p.RUnlock()
	require.False(t, plaintextIndexed)

	got, ok, err := p.GetContext(context.Background(), "U1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Alice", got.GetLogin())
	require.Equal(t, "Alice A.", got.GetDisplayName())
	require.Equal(t, 2, unwrapCalls, "one user hydration should reuse its DEK within the read")

	encryptedEmail, err := encryptUserPIIStringWithContentKey(contentKey, "E2", "U1", events.EventUserVerifiedEmailAdded, "email", "Alice@Example.com")
	require.NoError(t, err)
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E2",
		Event: &corev1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
			UserId:         "U1",
			EncryptedEmail: encryptedEmail,
		}},
	}, 3))
	require.Equal(t, 3, unwrapCalls, "projection apply decrypts email transiently to derive its lookup digest")
	p.RLock()
	projectedEmail := p.users["U1"].verifiedEmail[emailHash("Alice@Example.com")]
	require.NotEmpty(t, projectedEmail.pii.encrypted.GetEncryptedValue())
	p.RUnlock()

	byEmail, ok, err := p.GetByEmailContext(context.Background(), "alice@example.com")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "U1", byEmail.GetId())
	require.Equal(t, 4, unwrapCalls, "profile and email hydration should share one DEK unwrap")
}

func TestUserProjection_ReadErrorsDoNotBecomeAbsenceOrTombstones(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	require.NoError(t, p.Apply(userEvent("E1", time.Now(), accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))
	encryptedEmail, err := encryptUserPIIStringWithContentKey(contentKey, "E2", "U1", events.EventUserVerifiedEmailAdded, "email", "alice@example.com")
	require.NoError(t, err)
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E2",
		Event: &corev1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
			UserId:         "U1",
			EncryptedEmail: encryptedEmail,
		}},
	}, 3))
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E3",
		Event: &corev1.Event_UserExternalIdentityLinked{UserExternalIdentityLinked: &corev1.UserExternalIdentityLinkedEvent{
			UserId:      "U1",
			Issuer:      "https://issuer.example",
			Subject:     "subject-1",
			SubjectHash: externalIdentityHash("https://issuer.example", "subject-1"),
		}},
	}, 4))

	p.dekResolver.keyWrapper = staticProjectionKeyWrapper{unwrapErr: errors.New("KMS unavailable")}
	changed := loginChanged(t, contentKey, "E4", "U1", "Alice2")
	err = p.Apply(userEvent("E4", time.Now(), changed), 5)
	require.ErrorContains(t, err, "KMS unavailable")
	require.True(t, p.LoginExists("Alice"), "failed projection apply must preserve the prior lookup")
	require.False(t, p.LoginExists("Alice2"))
	user, ok, err := p.GetContext(context.Background(), "U1")
	require.ErrorContains(t, err, "KMS unavailable")
	require.False(t, ok)
	require.Nil(t, user)
	reference, ok, err := p.GetReferenceContext(context.Background(), "U1")
	require.ErrorContains(t, err, "KMS unavailable")
	require.False(t, ok)
	require.Nil(t, reference, "operational failures must not look like deleted users")
	ownerID, claimed := p.EmailOwnerID("alice@example.com")
	require.True(t, claimed, "uniqueness lookup must not depend on KMS availability")
	require.Equal(t, "U1", ownerID)
	ownerID, claimed = p.ExternalIdentityOwnerID("https://issuer.example", "subject-1")
	require.True(t, claimed, "identity uniqueness lookup must not depend on KMS availability")
	require.Equal(t, "U1", ownerID)
}

func TestUserProjection_LoginCooldownUsesEnvelopeTime(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	changedAt := createdAt.Add(5 * time.Minute)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))
	require.True(t, p.LoginChangedAt("U1").IsZero())

	require.NoError(t, p.Apply(userEvent("E3", changedAt, loginChanged(t, contentKey, "E3", "U1", "Alice2")), 3))
	require.True(t, p.LoginChangedAt("U1").IsZero())
	require.False(t, p.LoginExists("Alice"))
	require.True(t, p.LoginExists("alice2"))

	require.NoError(t, p.Apply(userEvent("E4", changedAt, loginCooldownStarted("U1")), 4))
	require.True(t, p.LoginChangedAt("U1").Equal(changedAt))

	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E5",
		Event: &corev1.Event_UserLoginCooldownCleared{UserLoginCooldownCleared: &corev1.UserLoginCooldownClearedEvent{UserId: "U1"}},
	}, 5))
	require.True(t, p.LoginChangedAt("U1").IsZero())
}

func TestUserProjection_CustomStatusSetClearAndExpiry(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E2",
		Event: &corev1.Event_UserCustomStatusSet{UserCustomStatusSet: &corev1.UserCustomStatusSetEvent{
			UserId: "U1",
			Status: &corev1.CustomUserStatus{
				Emoji:     "🌿",
				Text:      "In focus mode",
				ExpiresAt: timestamppb.New(future),
			},
		}},
	}, 3))

	got, ok := p.Get("U1")
	require.True(t, ok)
	require.Equal(t, "🌿", got.GetCustomStatus().GetEmoji())
	require.Equal(t, "In focus mode", got.GetCustomStatus().GetText())

	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E3",
		Event: &corev1.Event_UserCustomStatusSet{UserCustomStatusSet: &corev1.UserCustomStatusSetEvent{
			UserId: "U1",
			Status: &corev1.CustomUserStatus{
				Emoji:     "☕",
				Text:      "Coffee",
				ExpiresAt: timestamppb.New(past),
			},
		}},
	}, 4))

	got, ok = p.Get("U1")
	require.True(t, ok)
	require.Nil(t, got.GetCustomStatus())

	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E4",
		Event: &corev1.Event_UserCustomStatusSet{UserCustomStatusSet: &corev1.UserCustomStatusSetEvent{
			UserId: "U1",
			Status: &corev1.CustomUserStatus{
				Emoji: "✅",
				Text:  "Back",
			},
		}},
	}, 5))
	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E5",
		Event: &corev1.Event_UserCustomStatusCleared{UserCustomStatusCleared: &corev1.UserCustomStatusClearedEvent{UserId: "U1"}},
	}, 6))

	got, ok = p.Get("U1")
	require.True(t, ok)
	require.Nil(t, got.GetCustomStatus())
}

func TestUserProjection_VerifiedEmailAvatarOIDCAndDelete(t *testing.T) {
	p, contentKey := newEncryptedUserProjection(t, "U1")
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	verifiedAt := createdAt.Add(time.Hour)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A.")), 2))
	encryptedEmail, err := encryptUserPIIStringWithContentKey(contentKey, "E3", "U1", events.EventUserVerifiedEmailAdded, "email", "Alice@Example.com")
	require.NoError(t, err)
	require.NoError(t, p.Apply(&corev1.Event{
		Id:        "E3",
		CreatedAt: timestamppb.New(verifiedAt),
		Event: &corev1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
			UserId:         "U1",
			EncryptedEmail: encryptedEmail,
		}},
	}, 3))
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E4",
		Event: &corev1.Event_UserAvatarSet{UserAvatarSet: &corev1.UserAvatarSetEvent{
			UserId: "U1",
			Avatar: &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_S3{S3: &corev1.S3Asset{Key: "avatars/U1"}}},
		}},
	}, 4))
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E5",
		Event: &corev1.Event_UserOidcSubjectLinked{UserOidcSubjectLinked: &corev1.UserOIDCSubjectLinkedEvent{
			UserId:  "U1",
			Issuer:  "https://issuer.example",
			Subject: "subject-1",
		}},
	}, 5))

	emails := p.VerifiedEmails("U1")
	require.Len(t, emails, 1)
	require.Equal(t, "Alice@Example.com", emails[0].Email)
	require.True(t, emails[0].VerifiedAt.Equal(verifiedAt))
	byEmail, ok := p.GetByEmail("alice@example.com")
	require.True(t, ok)
	require.Equal(t, "U1", byEmail.GetId())
	byOIDC, ok := p.GetByOIDCSubject("https://issuer.example", "subject-1")
	require.True(t, ok)
	require.Equal(t, "U1", byOIDC.GetId())
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E5b",
		Event: &corev1.Event_UserExternalIdentityLinked{UserExternalIdentityLinked: &corev1.UserExternalIdentityLinkedEvent{
			UserId:       "U1",
			Issuer:       "github-main",
			Subject:      "12345",
			ProviderId:   "github-main",
			ProviderType: "github",
		}},
	}, 5))
	byExternal, ok := p.GetByExternalIdentity("github-main", "12345")
	require.True(t, ok)
	require.Equal(t, "U1", byExternal.GetId())
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E5c",
		Event: &corev1.Event_UserExternalIdentityUnlinked{UserExternalIdentityUnlinked: &corev1.UserExternalIdentityUnlinkedEvent{
			UserId:      "U1",
			SubjectHash: externalIdentityHash("github-main", "12345"),
		}},
	}, 5))
	_, ok = p.GetByExternalIdentity("github-main", "12345")
	require.False(t, ok)
	require.Len(t, p.ExternalIdentities("U1"), 1)
	avatar, ok := p.Avatar("U1")
	require.True(t, ok)
	require.Equal(t, "avatars/U1", avatar.GetS3().GetKey())

	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E6",
		Event: &corev1.Event_UserAvatarCleared{UserAvatarCleared: &corev1.UserAvatarClearedEvent{UserId: "U1"}},
	}, 6))
	_, ok = p.Avatar("U1")
	require.False(t, ok)

	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E7",
		Event: &corev1.Event_UserAccountDeleted{UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: "U1"}},
	}, 7))
	_, ok = p.Get("U1")
	require.False(t, ok)
	require.False(t, p.LoginExists("alice"))
	require.False(t, p.EmailClaimed("Alice@Example.com"))
	_, ok = p.GetByOIDCSubject("https://issuer.example", "subject-1")
	require.False(t, ok)
	_, ok = p.GetByExternalIdentity("github-main", "12345")
	require.False(t, ok)
}

func TestUserProjection_PublicAvatarIndexTracksLifecycle(t *testing.T) {
	p := NewUserProjection(staticProjectionKeyWrapper{}, staticProjectionDEKStore{})

	legacy := &corev1.Event{Event: &corev1.Event_UserAvatarSet{UserAvatarSet: &corev1.UserAvatarSetEvent{
		UserId: "U1",
		Avatar: &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_S3{S3: &corev1.S3Asset{Key: "legacy-avatar-key"}}},
	}}}
	replacement := &corev1.AssetRecord{
		Id:      "A-replacement",
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "replacement-storage-key"}},
	}

	require.NoError(t, p.Apply(legacy, 1))
	require.True(t, p.IsPublicAvatarAsset("legacy-avatar-key"))

	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_AssetCreated{AssetCreated: &corev1.AssetCreatedEvent{
		UserId: "U1",
		Asset:  replacement,
	}}}, 2))
	require.False(t, p.IsPublicAvatarAsset("legacy-avatar-key"))
	require.True(t, p.IsPublicAvatarAsset("A-replacement"))
	require.True(t, p.IsPublicAvatarAsset("replacement-storage-key"))

	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_AssetDeleted{AssetDeleted: &corev1.AssetDeletedEvent{
		AssetId: "A-replacement",
	}}}, 3))
	require.False(t, p.IsPublicAvatarAsset("A-replacement"))
	require.False(t, p.IsPublicAvatarAsset("replacement-storage-key"))

	require.NoError(t, p.Apply(legacy, 4))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_UserAvatarCleared{UserAvatarCleared: &corev1.UserAvatarClearedEvent{
		UserId: "U1",
	}}}, 5))
	require.False(t, p.IsPublicAvatarAsset("legacy-avatar-key"))

	require.NoError(t, p.Apply(legacy, 6))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_UserAccountDeleted{UserAccountDeleted: &corev1.UserAccountDeletedEvent{
		UserId: "U1",
	}}}, 7))
	require.False(t, p.IsPublicAvatarAsset("legacy-avatar-key"))
}
