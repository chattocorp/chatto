package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestUserKeyShreddingRequestIsTheFailClosedBoundary(t *testing.T) {
	chatto := setupTestCoreWithEncryption(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "shred-boundary", "Shred Boundary", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, user.GetId(), KindChannel, "", "shred-boundary", "")
	require.NoError(t, err)
	message, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), user.GetId(), "private", nil, "", "", nil, false)
	require.NoError(t, err)

	contentRefs, wrappingRefs, err := chatto.keyShredding.shreddingTargets(ctx, user.GetId())
	require.NoError(t, err)
	require.NotEmpty(t, contentRefs)
	require.NotEmpty(t, wrappingRefs)

	originalRequestAppend := chatto.keyShredding.appendRequestAtFn
	var failRequestAppend atomic.Bool
	failRequestAppend.Store(true)
	chatto.keyShredding.appendRequestAtFn = func(ctx context.Context, subject string, event *corev1.Event, filter string, expectedSeq uint64) (uint64, error) {
		if failRequestAppend.Load() {
			return 0, errors.New("injected request append failure")
		}
		return originalRequestAppend(ctx, subject, event, filter, expectedSeq)
	}

	err = chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId())
	require.ErrorContains(t, err, "injected request append failure")
	requestEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShreddingRequested))
	require.NoError(t, err)
	require.Empty(t, requestEvents)
	for _, ref := range contentRefs {
		_, err := chatto.encryption.contentKeys.Get(ctx, ref)
		require.NoError(t, err, "request publication failure must leave DEK %s intact", ref)
	}

	failRequestAppend.Store(false)
	originalShredContent := chatto.keyShredding.shredContentKeyFn
	var failContentShred atomic.Bool
	failContentShred.Store(true)
	chatto.keyShredding.shredContentKeyFn = func(ctx context.Context, ref string) error {
		if failContentShred.Load() {
			return errors.New("injected content-key shred failure")
		}
		return originalShredContent(ctx, ref)
	}

	err = chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId())
	require.ErrorContains(t, err, "injected content-key shred failure")
	requestEvents, _, err = chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShreddingRequested))
	require.NoError(t, err)
	require.Len(t, requestEvents, 1)
	request := requestEvents[0].GetUserKeyShreddingRequested()
	require.Equal(t, user.GetId(), request.GetUserId())

	completionEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShredded))
	require.NoError(t, err)
	require.Empty(t, completionEvents)
	body, err := chatto.GetFullMessageBody(ctx, message.GetId())
	require.NoError(t, err)
	require.Nil(t, body, "a committed request must tombstone content before physical deletion")
	_, err = chatto.ensureActiveUserDEK(ctx, user.GetId(), corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	require.ErrorIs(t, err, encryption.ErrKeyNotFound, "a committed request must prevent lazy key regeneration")
	for _, ref := range contentRefs {
		_, err := chatto.encryption.contentKeys.Get(ctx, ref)
		require.NoError(t, err, "injected failure must leave DEK %s intact", ref)
	}

	failContentShred.Store(false)
	require.NoError(t, chatto.DeleteUserEncryptionKeyAs(ctx, SystemActorID, user.GetId()))
	for _, ref := range contentRefs {
		_, err := chatto.encryption.contentKeys.Get(ctx, ref)
		require.ErrorIs(t, err, encryption.ErrKeyNotFound)
	}
	completionEvents, _, err = chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShredded))
	require.NoError(t, err)
	require.Len(t, completionEvents, 1)
	require.Equal(t, user.GetId(), completionEvents[0].GetActorId(), "completion must preserve the initiating request actor")
	require.NoError(t, chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId()))
	requestEvents, _, err = chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShreddingRequested))
	require.NoError(t, err)
	require.Len(t, requestEvents, 1)
}

func TestUserKeyShreddingDiscoversCoordinatesAfterAggregateConflict(t *testing.T) {
	chatto := setupTestCoreWithEncryption(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "shred-occ", "Shred OCC", "password123")
	require.NoError(t, err)

	aggregate := evtstream.UserAggregate(user.GetId())
	originalRequestAppend := chatto.keyShredding.appendRequestAtFn
	var inserted atomic.Bool
	var competingContentRef, competingWrappingRef string
	chatto.keyShredding.appendRequestAtFn = func(ctx context.Context, subject string, event *corev1.Event, filter string, expectedSeq uint64) (uint64, error) {
		if inserted.CompareAndSwap(false, true) {
			competingWrappingRef, err = chatto.encryption.keyWrapper.CreateKey(ctx, user.GetId())
			require.NoError(t, err)
			_, wrapped, wrapErr := chatto.newWrappedUserDEK(ctx, user.GetId(), competingWrappingRef, 2, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
			require.NoError(t, wrapErr)
			competingContentRef = wrapped.GetContentKeyRef()
			_, appendErr := chatto.EventPublisher.AppendAtFilter(ctx,
				aggregate.Subject(evtstream.EventUserDEKGenerated),
				newEvent(user.GetId(), &corev1.Event{Event: &corev1.Event_UserDekGenerated{UserDekGenerated: wrapped}}),
				filter,
				expectedSeq,
			)
			require.NoError(t, appendErr)
		}
		return originalRequestAppend(ctx, subject, event, filter, expectedSeq)
	}

	require.NoError(t, chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId()))
	requestEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, aggregate.Subject(evtstream.EventUserKeyShreddingRequested))
	require.NoError(t, err)
	require.Len(t, requestEvents, 1)
	require.Equal(t, user.GetId(), requestEvents[0].GetUserKeyShreddingRequested().GetUserId())
	_, err = chatto.encryption.contentKeys.Get(ctx, competingContentRef)
	require.ErrorIs(t, err, encryption.ErrKeyNotFound)
	exists, err := chatto.encryption.keyWrapper.KeyExists(ctx, competingWrappingRef)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteUserRequiresDurableKeyShreddingRequest(t *testing.T) {
	chatto := setupTestCoreWithEncryption(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "shred-delete", "Shred Delete", "password123")
	require.NoError(t, err)

	chatto.keyShredding.appendRequestAtFn = func(context.Context, string, *corev1.Event, string, uint64) (uint64, error) {
		return 0, errors.New("injected request append failure")
	}
	err = chatto.DeleteUser(ctx, user.GetId(), user.GetId())
	require.ErrorContains(t, err, "record durable user-key shredding request")
	_, err = chatto.GetUser(ctx, user.GetId())
	require.NoError(t, err, "account deletion must abort before its privacy boundary is durable")
	deletedEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserAccountDeleted))
	require.NoError(t, err)
	require.Empty(t, deletedEvents)
}

func TestUserKeyShreddingRetryRecordsCompletionAfterPhysicalSuccess(t *testing.T) {
	chatto := setupTestCoreWithEncryption(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "shred-completion", "Shred Completion", "password123")
	require.NoError(t, err)
	contentRefs, _, err := chatto.keyShredding.shreddingTargets(ctx, user.GetId())
	require.NoError(t, err)

	originalAppend := chatto.keyShredding.appendOnceFn
	var failCompletionAppend atomic.Bool
	failCompletionAppend.Store(true)
	chatto.keyShredding.appendOnceFn = func(ctx context.Context, userID string, event *corev1.Event, eventType string) (uint64, error) {
		if eventType == evtstream.EventUserKeyShredded && failCompletionAppend.Load() {
			return 0, errors.New("injected completion append failure")
		}
		return originalAppend(ctx, userID, event, eventType)
	}

	err = chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId())
	require.ErrorContains(t, err, "injected completion append failure")
	requestEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShreddingRequested))
	require.NoError(t, err)
	require.Len(t, requestEvents, 1)
	completionEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShredded))
	require.NoError(t, err)
	require.Empty(t, completionEvents)
	for _, ref := range contentRefs {
		_, err := chatto.encryption.contentKeys.Get(ctx, ref)
		require.ErrorIs(t, err, encryption.ErrKeyNotFound, "physical deletion must survive a missing completion fact")
	}

	failCompletionAppend.Store(false)
	require.NoError(t, chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId()))
	completionEvents, _, err = chatto.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserKeyShredded))
	require.NoError(t, err)
	require.Len(t, completionEvents, 1)
}

func TestUserKeyShreddingKeepsDEKsDiscoverableUntilWrappingKeysAreShredded(t *testing.T) {
	chatto := setupTestCoreWithEncryption(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "shred-order", "Shred Order", "password123")
	require.NoError(t, err)
	contentRefs, _, err := chatto.keyShredding.shreddingTargets(ctx, user.GetId())
	require.NoError(t, err)
	require.NotEmpty(t, contentRefs)

	originalShredWrapping := chatto.keyShredding.shredWrappingKeyFn
	chatto.keyShredding.shredWrappingKeyFn = func(context.Context, string) error {
		return errors.New("injected wrapping-key shred failure")
	}
	err = chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId())
	require.ErrorContains(t, err, "injected wrapping-key shred failure")
	for _, ref := range contentRefs {
		_, err := chatto.encryption.contentKeys.Get(ctx, ref)
		require.NoError(t, err, "DEK %s must remain discoverable while KEK shredding is incomplete", ref)
	}

	chatto.keyShredding.shredWrappingKeyFn = originalShredWrapping
	require.NoError(t, chatto.DeleteUserEncryptionKeyAs(ctx, user.GetId(), user.GetId()))
	for _, ref := range contentRefs {
		_, err := chatto.encryption.contentKeys.Get(ctx, ref)
		require.ErrorIs(t, err, encryption.ErrKeyNotFound)
	}
}
