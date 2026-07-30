package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNewUserModelWiresDependencies(t *testing.T) {
	publisher := testEventPublisher(t)
	users := NewUserProjection(nil, nil)
	usersProjector := testEventProjector(t)
	authProjector := testEventProjector(t)
	contentKeys := NewContentKeyProjection()
	contentKeysProjector := testEventProjector(t)

	service := newUserModel(publisher, users, usersProjector, authProjector, contentKeys, contentKeysProjector)

	if service.publisher != publisher {
		t.Fatal("publisher was not wired")
	}
	if service.users != users {
		t.Fatal("users projection was not wired")
	}
	if service.usersProjector != usersProjector {
		t.Fatal("users projector was not wired")
	}
	if service.authProjector != authProjector {
		t.Fatal("user auth projector was not wired")
	}
	if service.contentKeys != contentKeys {
		t.Fatal("content keys projection was not wired")
	}
	if service.contentKeysProjector != contentKeysProjector {
		t.Fatal("content keys projector was not wired")
	}
}

func TestUserModelWaitForContentKeysProjectsDEKGenerated(t *testing.T) {
	harness := newTestEventHarness(t)
	contentKeys := NewContentKeyProjection()
	contentKeysProjector := harness.projector(contentKeys)
	startTestProjector(t, contentKeysProjector)
	service := newUserModel(harness.publisher, nil, nil, nil, contentKeys, contentKeysProjector)
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
	subject := events.UserAggregate("U-service").SubjectFor(event)
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
	service := newUserModel(harness.publisher, users, usersProjector, nil, nil, nil)
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
	subject := events.UserAggregate("U-avatar").SubjectFor(event)
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
	service := newUserModel(harness.publisher, users, usersProjector, nil, contentKeys, contentKeysProjector)
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
	avatarSubject := events.UserAggregate("U-current").SubjectFor(avatarEvent)
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
	if _, err := harness.publisher.AppendEventually(ctx, events.UserAggregate("U-current").SubjectFor(dekEvent), dekEvent); err != nil {
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
	service := newUserModel(nil, nil, nil, nil, contentKeys, nil)
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
