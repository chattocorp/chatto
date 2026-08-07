package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func TestNewUserModelWiresDependencies(t *testing.T) {
	publisher := testEventPublisher(t)
	users := NewUserProjection(nil, nil)
	auth := users.AuthProjection()
	contentKeys := NewContentKeyProjection()
	usersHandle := detachedTestProjectionHandle(users)
	authHandle := detachedTestProjectionHandle(auth)
	contentKeysHandle := detachedTestProjectionHandle(contentKeys)

	service := newUserModel(publisher, usersHandle, authHandle, contentKeysHandle)

	if service.publisher != publisher {
		t.Fatal("publisher was not wired")
	}
	if service.users.Projection() != users {
		t.Fatal("users projection was not wired")
	}
	if service.users.Projector() != usersHandle.Projector() {
		t.Fatal("users projector was not wired")
	}
	if service.auth.Projection() != auth {
		t.Fatal("user auth projection was not wired")
	}
	if service.auth.Projector() != authHandle.Projector() {
		t.Fatal("user auth projector was not wired")
	}
	if service.contentKeys.Projection() != contentKeys {
		t.Fatal("content keys projection was not wired")
	}
	if service.contentKeys.Projector() != contentKeysHandle.Projector() {
		t.Fatal("content keys projector was not wired")
	}
}

func TestUserModelOwnsProfileAndAuthenticationReads(t *testing.T) {
	users, contentKey := newEncryptedUserProjection(t, "U1")
	auth := users.AuthProjection()
	model := newTestUserModel(t, nil, users, nil, auth, nil, nil, nil)
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	account := userEvent("E1", createdAt, accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A."))
	require.NoError(t, users.Apply(account, 2))
	require.NoError(t, auth.Apply(account, 2))

	encryptedEmail, err := encryptUserPIIStringWithContentKey(
		contentKey,
		"E2",
		"U1",
		evtstream.EventUserVerifiedEmailAdded,
		"email",
		"alice@example.com",
	)
	require.NoError(t, err)
	require.NoError(t, users.Apply(&corev1.Event{
		Id:        "E2",
		CreatedAt: timestamppb.New(createdAt.Add(time.Minute)),
		Event: &corev1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
				UserId:         "U1",
				EncryptedEmail: encryptedEmail,
			},
		},
	}, 3))
	require.NoError(t, users.Apply(&corev1.Event{
		Id: "E3",
		Event: &corev1.Event_UserAvatarSet{
			UserAvatarSet: &corev1.UserAvatarSetEvent{
				UserId: "U1",
				Avatar: &corev1.DeprecatedAsset{
					Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "avatar-U1"}},
				},
			},
		},
	}, 4))

	passwordAt := createdAt.Add(2 * time.Minute)
	require.NoError(t, auth.Apply(userEvent("E4", passwordAt, &corev1.Event{
		Event: &corev1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
				UserId:       "U1",
				PasswordHash: []byte("password-hash"),
			},
		},
	}), 5))
	require.NoError(t, auth.Apply(&corev1.Event{
		Id: "E5",
		Event: &corev1.Event_UserExternalIdentityLinked{
			UserExternalIdentityLinked: &corev1.UserExternalIdentityLinkedEvent{
				UserId:       "U1",
				Issuer:       "github",
				Subject:      "alice",
				ProviderId:   "github",
				ProviderType: "oauth",
			},
		},
	}, 6))
	require.NoError(t, auth.Apply(&corev1.Event{
		Id: "E6",
		Event: &corev1.Event_OauthConsentGranted{
			OauthConsentGranted: &corev1.OAuthConsentGrantedEvent{
				UserId:         "U1",
				RedirectOrigin: "https://app.example",
			},
		},
	}, 7))

	user, ok, err := model.user(context.Background(), "U1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Alice", user.GetLogin())
	user.Login = "mutated"
	userAgain, ok, err := model.user(context.Background(), "U1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Alice", userAgain.GetLogin(), "profile reads must be detached")

	byLogin, ok, err := model.userByLogin(context.Background(), "alice")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "U1", byLogin.GetId())
	byEmail, ok, err := model.userByEmail(context.Background(), "ALICE@example.com")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "U1", byEmail.GetId())
	byIdentity, ok, err := model.userByExternalIdentity(context.Background(), "github", "alice")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "U1", byIdentity.GetId())

	require.True(t, model.loginExists("ALICE"))
	require.True(t, model.emailClaimed("ALICE@example.com"))
	emailOwnerID, claimed := model.emailOwnerID("alice@example.com")
	require.True(t, claimed)
	require.Equal(t, "U1", emailOwnerID)
	identityOwnerID, claimed := model.externalIdentityOwnerID("github", "alice")
	require.True(t, claimed)
	require.Equal(t, "U1", identityOwnerID)
	require.Len(t, model.externalIdentities("U1"), 1)
	require.True(t, model.hasVerifiedEmail("U1"))
	require.True(t, model.hasVerifiedFactor("U1"))
	require.True(t, model.hasOAuthConsent("U1", "https://app.example"))
	require.Equal(t, []string{"U1"}, model.verifiedUserIDs())
	require.Equal(t, []string{"U1"}, model.verifiedAccountIDs())
	require.Equal(t, 1, model.userCount())

	emails, err := model.verifiedEmails(context.Background(), "U1")
	require.NoError(t, err)
	require.Equal(t, "alice@example.com", emails[0].Email)
	allUsers, err := model.allUsers(context.Background())
	require.NoError(t, err)
	require.Len(t, allUsers, 1)

	avatar, ok := model.avatar("U1")
	require.True(t, ok)
	require.Equal(t, "avatar-U1", avatar.GetId())
	avatar.Id = "mutated"
	avatarAgain, ok := model.avatar("U1")
	require.True(t, ok)
	require.Equal(t, "avatar-U1", avatarAgain.GetId(), "avatar reads must be detached")
	require.True(t, model.isPublicAvatarAsset("avatar-U1"))

	hash, setAt, ok := model.passwordHashWithSetAt("U1")
	require.True(t, ok)
	require.Equal(t, passwordAt, setAt)
	hash[0] = 'X'
	hashAgain, ok := model.passwordHash("U1")
	require.True(t, ok)
	require.Equal(t, []byte("password-hash"), hashAgain, "credential reads must be detached")
	generation, active := model.authGeneration("U1")
	require.True(t, active)
	require.Equal(t, uint64(5), generation)
}

func TestUserModelPreservesPIIFailuresAndShreddedReferences(t *testing.T) {
	users, contentKey := newEncryptedUserProjection(t, "U1")
	auth := users.AuthProjection()
	model := newTestUserModel(t, nil, users, nil, auth, nil, nil, nil)
	require.NoError(t, users.Apply(userEvent(
		"E1",
		time.Now(),
		accountCreated(t, contentKey, "E1", "U1", "Alice", "Alice A."),
	), 2))

	users.dekResolver.keyWrapper = staticProjectionKeyWrapper{unwrapErr: errors.New("KMS unavailable")}
	user, ok, err := model.user(context.Background(), "U1")
	require.ErrorContains(t, err, "KMS unavailable")
	require.False(t, ok)
	require.Nil(t, user)
	reference, ok, err := model.userReference(context.Background(), "U1")
	require.ErrorContains(t, err, "KMS unavailable")
	require.False(t, ok)
	require.Nil(t, reference, "operational failures must not look like deletion")

	users.dekResolver.keyWrapper = staticProjectionKeyWrapper{key: contentKey.key}
	require.NoError(t, users.Apply(&corev1.Event{
		Id: "E2",
		Event: &corev1.Event_UserKeyShredded{
			UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: "U1"},
		},
	}, 3))
	reference, ok, err = model.userReference(context.Background(), "U1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "U1", reference.GetId())
	require.True(t, reference.GetDeleted())
}

func TestUserModelWaitForContentKeysProjectsDEKGenerated(t *testing.T) {
	harness := newTestEventHarness(t)
	contentKeys := NewContentKeyProjection()
	contentKeysProjector := harness.projector(contentKeys)
	startTestProjector(t, contentKeysProjector)
	service := newTestUserModel(t, harness.publisher, nil, nil, nil, nil, contentKeys, contentKeysProjector)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: &corev1.UserDEKGeneratedEvent{
				UserId:         "U-service",
				Purpose:        corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
				Epoch:          2,
				ContentKeyRef:  "content-key-ref",
				WrappingKeyRef: "wrapping-key-ref",
			},
		},
	})
	subject := evtstream.UserAggregate("U-service").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForContentKeys(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForContentKeys returned error: %v", err)
	}

	active, ok, err := service.activeContentKey("U-service", corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	if err != nil {
		t.Fatalf("activeContentKey returned error: %v", err)
	}
	if !ok {
		t.Fatal("content key projection did not contain appended DEK")
	}
	if active.GetContentKeyRef() != "content-key-ref" {
		t.Fatalf("ContentKeyRef = %q, want %q", active.GetContentKeyRef(), "content-key-ref")
	}
}

func TestUserModelWaitForUsersProjectsUserAvatar(t *testing.T) {
	harness := newTestEventHarness(t)
	users := NewUserProjection(nil, nil)
	usersProjector := harness.projector(users)
	startTestProjector(t, usersProjector)
	service := newTestUserModel(t, harness.publisher, users, usersProjector, users.AuthProjection(), nil, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_UserAvatarSet{
			UserAvatarSet: &corev1.UserAvatarSetEvent{
				UserId: "U-avatar",
				Avatar: &corev1.DeprecatedAsset{
					Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "avatar-asset"}},
				},
			},
		},
	})
	subject := evtstream.UserAggregate("U-avatar").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForUsers(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForUsers returned error: %v", err)
	}

	avatar, ok := users.Avatar("U-avatar")
	if !ok {
		t.Fatal("user projection did not contain projected avatar")
	}
	if avatar.GetId() != "avatar-asset" {
		t.Fatalf("avatar id = %q, want %q", avatar.GetId(), "avatar-asset")
	}
}

func TestUserModelCurrentWaitsUsePublisherTail(t *testing.T) {
	harness := newTestEventHarness(t)
	users := NewUserProjection(nil, nil)
	usersProjector := harness.projector(users)
	startTestProjector(t, usersProjector)
	contentKeys := NewContentKeyProjection()
	contentKeysProjector := harness.projector(contentKeys)
	startTestProjector(t, contentKeysProjector)
	service := newTestUserModel(t, harness.publisher, users, usersProjector, users.AuthProjection(), nil, contentKeys, contentKeysProjector)
	ctx := testContext(t)

	avatarEvent := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_UserAvatarSet{
			UserAvatarSet: &corev1.UserAvatarSetEvent{
				UserId: "U-current",
				Avatar: &corev1.DeprecatedAsset{
					Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "avatar-current"}},
				},
			},
		},
	})
	avatarSubject := evtstream.UserAggregate("U-current").SubjectFor(avatarEvent)
	if _, err := harness.publisher.AppendEventually(ctx, avatarSubject, avatarEvent); err != nil {
		t.Fatalf("AppendEventually avatar returned error: %v", err)
	}
	if err := service.waitForUsersCurrent(ctx, "users", avatarSubject); err != nil {
		t.Fatalf("waitForUsersCurrent returned error: %v", err)
	}
	if avatar, ok := users.Avatar("U-current"); !ok || avatar.GetId() != "avatar-current" {
		t.Fatalf("projected avatar = %#v, %v; want avatar-current, true", avatar, ok)
	}

	dekEvent := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: &corev1.UserDEKGeneratedEvent{
				UserId:        "U-current",
				Purpose:       corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
				Epoch:         3,
				ContentKeyRef: "content-current",
			},
		},
	})
	if _, err := harness.publisher.AppendEventually(ctx, evtstream.UserAggregate("U-current").SubjectFor(dekEvent), dekEvent); err != nil {
		t.Fatalf("AppendEventually DEK returned error: %v", err)
	}
	if err := service.waitForContentKeysCurrent(ctx, "U-current"); err != nil {
		t.Fatalf("waitForContentKeysCurrent returned error: %v", err)
	}
	if active, ok, err := service.activeContentKey("U-current", corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY); err != nil || !ok || active.GetContentKeyRef() != "content-current" {
		if err != nil {
			t.Fatalf("activeContentKey returned error: %v", err)
		}
		t.Fatalf("projected content key = %#v, %v; want content-current, true", active, ok)
	}
}

func TestUserModelContentKeyReadsPreserveProjectionSemantics(t *testing.T) {
	contentKeys := NewContentKeyProjection()
	service := newTestUserModel(t, nil, nil, nil, nil, nil, contentKeys, nil)
	legacy := &corev1.UserDEKGeneratedEvent{
		UserId:         "U-legacy",
		Epoch:          2,
		ContentKeyRef:  "content-legacy",
		WrappingKeyRef: "wrapping-legacy",
	}
	if err := contentKeys.Apply(&corev1.Event{
		Id: "E-legacy",
		Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: legacy,
		},
	}, 1); err != nil {
		t.Fatalf("Apply legacy DEK: %v", err)
	}

	purpose := corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY
	active, ok, err := service.activeContentKey("U-legacy", purpose)
	if err != nil {
		t.Fatalf("activeContentKey returned error: %v", err)
	}
	if !ok || active.GetContentKeyRef() != "content-legacy" {
		t.Fatalf("active content key = %#v, %v; want legacy fallback", active, ok)
	}
	atEpoch, ok, err := service.contentKeyAtEpoch("U-legacy", purpose, 2)
	if err != nil {
		t.Fatalf("contentKeyAtEpoch returned error: %v", err)
	}
	if !ok || atEpoch.GetContentKeyRef() != "content-legacy" {
		t.Fatalf("content key at epoch = %#v, %v; want legacy fallback", atEpoch, ok)
	}
	contentKeyRefs, wrappingKeyRefs, err := service.keyRefsForShredding("U-legacy")
	if err != nil {
		t.Fatalf("keyRefsForShredding returned error: %v", err)
	}
	if len(contentKeyRefs) != 1 || contentKeyRefs[0] != "content-legacy" {
		t.Fatalf("content key refs = %v, want [content-legacy]", contentKeyRefs)
	}
	if len(wrappingKeyRefs) != 1 || wrappingKeyRefs[0] != "wrapping-legacy" {
		t.Fatalf("wrapping key refs = %v, want [wrapping-legacy]", wrappingKeyRefs)
	}
}

func TestUserModelCurrentWaitsAreNoopsWhenDependenciesMissing(t *testing.T) {
	ctx := testContext(t)
	service := &UserModel{}

	if service.isPublicAvatarAsset("avatar-U1") {
		t.Fatal("missing user projection classified an avatar as public")
	}
	if err := service.waitForUsersCurrent(ctx, "users", "evt.user.U1.created"); err != nil {
		t.Fatalf("waitForUsersCurrent returned error: %v", err)
	}
	if err := service.waitForContentKeysCurrent(ctx, "U1"); err != nil {
		t.Fatalf("waitForContentKeysCurrent returned error: %v", err)
	}
	if _, _, err := service.activeContentKey("U1", corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY); !errors.Is(err, errContentKeyProjectionUnavailable) {
		t.Fatalf("activeContentKey error = %v, want %v", err, errContentKeyProjectionUnavailable)
	}
	if _, _, err := service.contentKeyAtEpoch("U1", corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, 1); !errors.Is(err, errContentKeyProjectionUnavailable) {
		t.Fatalf("contentKeyAtEpoch error = %v, want %v", err, errContentKeyProjectionUnavailable)
	}
	if _, _, err := service.keyRefsForShredding("U1"); !errors.Is(err, errContentKeyProjectionUnavailable) {
		t.Fatalf("keyRefsForShredding error = %v, want %v", err, errContentKeyProjectionUnavailable)
	}
}
