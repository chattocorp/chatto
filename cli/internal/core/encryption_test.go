package core

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/signedurl"
)

type countingKeyWrapper struct {
	kms.KeyWrapper
	unwraps atomic.Int32
}

func (w *countingKeyWrapper) UnwrapContentKey(ctx context.Context, keyRef string, wrapped kms.WrappedContentKey, aad []byte) ([]byte, error) {
	w.unwraps.Add(1)
	return w.KeyWrapper.UnwrapContentKey(ctx, keyRef, wrapped, aad)
}

type delayedKeyWrapper struct {
	kms.KeyWrapper
	delay time.Duration
}

func (w delayedKeyWrapper) UnwrapContentKey(ctx context.Context, keyRef string, wrapped kms.WrappedContentKey, aad []byte) ([]byte, error) {
	time.Sleep(w.delay)
	return w.KeyWrapper.UnwrapContentKey(ctx, keyRef, wrapped, aad)
}

// setupTestCoreWithEncryption creates a ChattoCore for encryption tests.
func setupTestCoreWithEncryption(t testing.TB) *ChattoCore {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	core, err := NewChattoCore(ctx, nc, cfg)
	require.NoError(t, err)

	startCoreServices(t, core)

	return core
}

func userDEKEventByPurpose(t *testing.T, eventList []*corev1.Event, purpose corev1.UserDEKPurpose) *corev1.UserDEKGeneratedEvent {
	t.Helper()
	for _, event := range eventList {
		dek := event.GetUserDekGenerated()
		if dek != nil && dek.GetPurpose() == purpose {
			return dek
		}
	}
	require.FailNowf(t, "missing DEK event", "purpose %s not found", purpose.String())
	return nil
}

func TestPostMessage_EncryptsMessageBody(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// Post a message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NotNil(t, event)

	stored, retracted, ok := core.RoomTimeline.LatestBody(event.Id)
	require.True(t, ok, "expected message to be projected")
	require.False(t, retracted, "new message should not be retracted")
	require.NotNil(t, stored, "expected projected body from MessageBodyEvent")
	require.NotEmpty(t, stored.EncryptedBody, "encrypted body should not be empty")
	require.NotEmpty(t, stored.EncryptionNonce, "nonce should not be empty")
	require.Equal(t, encryption.EnvelopeVersionV2, stored.EncryptionVersion, "new messages should use v2 envelopes")
	require.EqualValues(t, 1, stored.ContentKeyEpoch, "new messages should reference the active message-body DEK epoch")
	require.NotEmpty(t, stored.BodyEventId, "new body-event-carried bodies should bind to their body event")

	contentKeyEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventUserDEKGenerated))
	require.NoError(t, err)
	require.Len(t, contentKeyEvents, 2)
	contentKeyEvent := userDEKEventByPurpose(t, contentKeyEvents, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	require.NotNil(t, contentKeyEvent)
	require.Equal(t, user.Id, contentKeyEvent.GetUserId())
	require.EqualValues(t, 1, contentKeyEvent.GetEpoch())
	require.Equal(t, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, contentKeyEvent.GetPurpose())
	require.NotEmpty(t, contentKeyEvent.GetContentKeyRef(), "DEK event should reference DEK storage")
	require.NotEmpty(t, contentKeyEvent.GetWrappingKeyRef(), "DEK event should reference the wrapping KEK")
	require.NotEqual(t, kms.LegacyUserKeyRef(user.Id), contentKeyEvent.GetWrappingKeyRef(), "new users should use opaque KMS key refs")
	storedContentKey, err := core.encryption.contentKeys.Get(ctx, contentKeyEvent.GetContentKeyRef())
	require.NoError(t, err)
	require.NotEmpty(t, storedContentKey.GetEncryptedContentKey(), "wrapped DEK should be stored in DEK storage")
	require.NotEmpty(t, storedContentKey.GetContentKeyNonce(), "wrapped DEK nonce should be stored in DEK storage")
	require.Equal(t, contentKeyEvent.GetWrappingKeyRef(), storedContentKey.GetWrappingKeyRef())
	_, err = core.storage.runtimeStateKV.Get(ctx, contentKeyEvent.GetContentKeyRef())
	require.NoError(t, err, "wrapped DEK should be stored in RUNTIME_STATE")
	_, err = core.storage.encryptionKV.Get(ctx, contentKeyEvent.GetContentKeyRef())
	require.Error(t, err, "wrapped DEK should not be stored in ENCRYPTION_KEYS")

	// Verify we can read the message back (decrypted)
	body, err := core.GetMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body, "decrypted message should match original")
}

func TestMessageBodyV2AADRejectsWrongEventContext(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "aaduser", "AAD User", "password123")
	require.NoError(t, err)
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Bound to one event", nil, "", "", nil, false)
	require.NoError(t, err)
	stored, retracted, ok := core.RoomTimeline.LatestBody(event.Id)
	require.True(t, ok)
	require.False(t, retracted)
	require.NotNil(t, stored)

	plaintext, err := core.decryptMessageBody(ctx, event.Id, room.Id, stored)
	require.NoError(t, err)
	require.Equal(t, "Bound to one event", string(plaintext))

	_, err = core.decryptMessageBody(ctx, "EtamperedEventID", room.Id, stored)
	require.ErrorIs(t, err, encryption.ErrDecryptionFailed)

	_, err = core.decryptMessageBody(ctx, event.Id, "RtamperedRoomID", stored)
	require.ErrorIs(t, err, encryption.ErrDecryptionFailed)

	t.Run("tampered body event ID", func(t *testing.T) {
		tampered := proto.Clone(stored).(*corev1.MessageBody)
		tampered.BodyEventId = "EtamperedBodyEventID"
		_, err := core.decryptMessageBody(ctx, event.Id, room.Id, tampered)
		require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
	})

	t.Run("tampered body nonce", func(t *testing.T) {
		tampered := proto.Clone(stored).(*corev1.MessageBody)
		tampered.EncryptionNonce = append([]byte(nil), tampered.GetEncryptionNonce()...)
		tampered.EncryptionNonce[0] ^= 0xff
		_, err := core.decryptMessageBody(ctx, event.Id, room.Id, tampered)
		require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
	})

	t.Run("tampered ciphertext", func(t *testing.T) {
		tampered := proto.Clone(stored).(*corev1.MessageBody)
		tampered.EncryptedBody = append([]byte(nil), tampered.GetEncryptedBody()...)
		tampered.EncryptedBody[0] ^= 0xff
		_, err := core.decryptMessageBody(ctx, event.Id, room.Id, tampered)
		require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
	})
}

func TestUserPIIEvents_AreEncryptedAndProjectable(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "piiuser", "PII User", "password123")
	require.NoError(t, err)
	require.NoError(t, core.AddVerifiedEmailDirect(ctx, user.Id, "pii@example.com"))

	updated, err := core.UpdateUserDisplayName(ctx, user.Id, "Renamed User")
	require.NoError(t, err)
	require.Equal(t, "Renamed User", updated.GetDisplayName())
	updated, err = core.UpdateUserLogin(ctx, user.Id, "piiuser2")
	require.NoError(t, err)
	require.Equal(t, "piiuser2", updated.GetLogin())

	accountEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventUserAccountCreated))
	require.NoError(t, err)
	require.Len(t, accountEvents, 1)
	contentKeyEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventUserDEKGenerated))
	require.NoError(t, err)
	require.Len(t, contentKeyEvents, 2)
	require.Equal(t, kms.AlgorithmBuiltinXChaCha20Poly1305V1, userDEKEventByPurpose(t, contentKeyEvents, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII).GetWrappingAlgorithm())
	account := accountEvents[0].GetUserAccountCreated()
	require.NotNil(t, account.GetEncryptedLogin())
	require.NotNil(t, account.GetEncryptedDisplayName())

	emailEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventUserVerifiedEmailAdded))
	require.NoError(t, err)
	require.Len(t, emailEvents, 1)
	email := emailEvents[0].GetUserVerifiedEmailAdded()
	require.NotNil(t, email.GetEncryptedEmail())

	displayNameEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventUserDisplayNameChanged))
	require.NoError(t, err)
	require.Len(t, displayNameEvents, 1)
	displayName := displayNameEvents[0].GetUserDisplayNameChanged()
	require.NotNil(t, displayName.GetEncryptedDisplayName())

	loginEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventUserLoginChanged))
	require.NoError(t, err)
	require.Len(t, loginEvents, 1)
	login := loginEvents[0].GetUserLoginChanged()
	require.NotNil(t, login.GetEncryptedLogin())

	found, err := core.GetUserByLogin(ctx, "piiuser2")
	require.NoError(t, err)
	require.Equal(t, user.Id, found.GetId())
	require.True(t, core.Users.EmailClaimed("pii@example.com"))
	emails := core.Users.VerifiedEmails(user.Id)
	require.Len(t, emails, 1)
	require.Equal(t, "pii@example.com", emails[0].Email)
}

func TestUserPIIProjection_ColdReplayAfterShredSkipsPIIIndexes(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "shreddedpii", "Shredded PII", "password123")
	require.NoError(t, err)
	require.NoError(t, core.AddVerifiedEmailDirect(ctx, user.Id, "shredded-pii@example.com"))
	require.NoError(t, core.DeleteUser(ctx, user.Id, user.Id))

	userEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).AllEventsFilter())
	require.NoError(t, err)

	replayed := NewUserProjection(core.encryption.keyWrapper, core.encryption.contentKeys)
	for i, event := range userEvents {
		require.NoError(t, replayed.Apply(event, uint64(i+1)))
	}
	require.False(t, replayed.LoginExists("shreddedpii"))
	require.False(t, replayed.EmailClaimed("shredded-pii@example.com"))
	got, ok := replayed.Get(user.Id)
	require.False(t, ok, "shredded user should not replay as a readable profile")
	require.Nil(t, got)
}

func TestUserPIIAADRejectsWrongContext(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	contentKey := &messageContentKey{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}

	encrypted, err := encryptUserPIIStringWithContentKey(contentKey, "E1", "U1", events.EventUserLoginChanged, "login", "alice")
	require.NoError(t, err)

	_, err = decryptUserPIIString(key, "E2", "U1", events.EventUserLoginChanged, "login", encrypted)
	require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
	_, err = decryptUserPIIString(key, "E1", "U1", events.EventUserLoginChanged, "display_name", encrypted)
	require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
}

func TestGetMessageBody_ReusesRequestCachedMessageBodyDEK(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "cacheuser", "Cache User", "password123")
	require.NoError(t, err)
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)

	wrapper := &countingKeyWrapper{KeyWrapper: core.encryption.keyWrapper}
	core.dekResolver.keyWrapper = wrapper

	readCtx := WithDEKRequestCache(ctx)
	body, err := core.GetMessageBody(readCtx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body)
	body, err = core.GetMessageBody(readCtx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body)
	require.EqualValues(t, 1, wrapper.unwraps.Load(), "message body DEK should be unwrapped once and then served from cache")
}

func TestUnwrappedDEKResolver_RequestCacheDoesNotPersistAcrossContexts(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	wrapper := &countingKeyWrapper{KeyWrapper: staticProjectionKeyWrapper{key: key}}
	resolver := newUnwrappedDEKResolver(wrapper, staticProjectionDEKStore{})
	event := &corev1.UserDEKGeneratedEvent{
		UserId:        "U-cache",
		Epoch:         1,
		Purpose:       corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		ContentKeyRef: "dek.test",
	}

	ctx := WithDEKRequestCache(context.Background())
	dek, err := resolver.Resolve(ctx, event, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	require.NoError(t, err)
	require.Equal(t, key, dek.key)
	dek, err = resolver.Resolve(ctx, event, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	require.NoError(t, err)
	require.Equal(t, key, dek.key)
	require.EqualValues(t, 1, wrapper.unwraps.Load())

	dek, err = resolver.Resolve(WithDEKRequestCache(context.Background()), event, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	require.NoError(t, err)
	require.Equal(t, key, dek.key)
	require.EqualValues(t, 2, wrapper.unwraps.Load())
}

func TestUnwrappedDEKResolver_RequestCacheCollapsesConcurrentMisses(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	wrapper := &countingKeyWrapper{KeyWrapper: delayedKeyWrapper{
		KeyWrapper: staticProjectionKeyWrapper{key: key},
		delay:      25 * time.Millisecond,
	}}
	resolver := newUnwrappedDEKResolver(wrapper, staticProjectionDEKStore{})
	event := &corev1.UserDEKGeneratedEvent{
		UserId:        "U-concurrent-cache",
		Epoch:         1,
		Purpose:       corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		ContentKeyRef: "dek.test",
	}

	ctx := WithDEKRequestCache(context.Background())
	const goroutineCount = 16
	start := make(chan struct{})
	errs := make(chan error, goroutineCount)
	var wg sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			dek, err := resolver.Resolve(ctx, event, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
			if err != nil {
				errs <- err
				return
			}
			if dek == nil || !bytes.Equal(key, dek.key) {
				errs <- fmt.Errorf("resolved wrong DEK")
				return
			}
			errs <- nil
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, wrapper.unwraps.Load(), "concurrent misses for one request cache key should share one unwrap")
}

func TestDeleteUserEncryptionKeyAsInvalidatesRequestCachedDEK(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "requestcachedelete", "Request Cache Delete", "password123")
	require.NoError(t, err)
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)

	readCtx := WithDEKRequestCache(ctx)
	body, err := core.GetMessageBody(readCtx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body)

	require.NoError(t, core.DeleteUserEncryptionKeyAs(readCtx, user.Id, user.Id))

	body, err = core.GetMessageBody(readCtx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Empty(t, body, "same request context should not keep using a DEK after deleting it")
}

func TestGetMessageBody_CryptoShredding(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// Post an encrypted message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)

	// Verify message can be read before key deletion
	body, err := core.GetMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body)

	// Delete the user's DEK records through the core path.
	require.NoError(t, core.DeleteUserEncryptionKeyAs(ctx, user.Id, user.Id))

	// Verify message now returns empty string (crypto-shredded)
	body, err = core.GetMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Empty(t, body, "message should be empty after crypto-shredding")

	// Also test GetFullMessageBody - returns nil for crypto-shredded (same as deleted)
	fullBody, err := core.GetFullMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Nil(t, fullBody, "message should be nil after crypto-shredding (treated same as deleted)")
}

func TestDeleteUserEncryptionKey_UsesStoredDEKWrappingRefs(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "storeddekref", "Stored DEK Ref", "password123")
	require.NoError(t, err)
	contentKeyEvent, ok := core.ContentKeys.Active(user.Id, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	require.True(t, ok)
	evtWrappingKeyRef := contentKeyEvent.GetWrappingKeyRef()
	require.NotEmpty(t, evtWrappingKeyRef)

	storedWrappingKeyRef, err := core.encryption.keyWrapper.CreateKey(ctx, user.Id)
	require.NoError(t, err)
	stored, err := core.encryption.contentKeys.Get(ctx, contentKeyEvent.GetContentKeyRef())
	require.NoError(t, err)
	stored.WrappingKeyRef = storedWrappingKeyRef
	data, err := proto.Marshal(stored)
	require.NoError(t, err)
	_, err = core.storage.runtimeStateKV.Put(ctx, contentKeyEvent.GetContentKeyRef(), data)
	require.NoError(t, err)

	require.NoError(t, core.DeleteUserEncryptionKeyAs(ctx, user.Id, user.Id))

	exists, err := core.encryption.keyWrapper.KeyExists(ctx, evtWrappingKeyRef)
	require.NoError(t, err)
	require.False(t, exists, "EVT wrapping key ref should be shredded")
	exists, err = core.encryption.keyWrapper.KeyExists(ctx, storedWrappingKeyRef)
	require.NoError(t, err)
	require.False(t, exists, "stored DEK wrapping key ref should also be shredded")
}

func TestDeleteUserEncryptionKey_ShredsLegacyUserKeyRef(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "legacyuserkey", "Legacy User Key", "password123")
	require.NoError(t, err)
	legacyKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	legacyRef := kms.LegacyUserKeyRef(user.Id)
	_, err = core.storage.encryptionKV.Create(ctx, legacyRef, legacyKey)
	require.NoError(t, err)

	require.NoError(t, core.DeleteUserEncryptionKeyAs(ctx, user.Id, user.Id))

	exists, err := core.encryption.keyWrapper.KeyExists(ctx, legacyRef)
	require.NoError(t, err)
	require.False(t, exists, "legacy user key ref should always be included in deletion")
}

func TestDeleteUserEncryptionKey_RejectsKEKContentKeyRef(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "badcontentref", "Bad Content Ref", "password123")
	require.NoError(t, err)
	mistypedContentRef, err := core.encryption.keyWrapper.CreateKey(ctx, user.Id)
	require.NoError(t, err)
	event := newEvent(user.Id, &corev1.Event{Event: &corev1.Event_UserDekGenerated{
		UserDekGenerated: &corev1.UserDEKGeneratedEvent{
			UserId:            user.Id,
			Epoch:             2,
			Purpose:           corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
			ContentKeyRef:     mistypedContentRef,
			WrappingAlgorithm: kms.AlgorithmBuiltinXChaCha20Poly1305V1,
			WrappingKeyRef:    mistypedContentRef,
		},
	}})
	seq, err := core.appendUserEvent(ctx, user.Id, event, "", nil)
	require.NoError(t, err)
	require.NoError(t, core.ContentKeysProjector.WaitFor(ctx, events.SubjectPosition(events.UserAggregate(user.Id).SubjectFor(event), seq)))

	err = core.DeleteUserEncryptionKeyAs(ctx, user.Id, user.Id)
	require.ErrorIs(t, err, dekstore.ErrInvalidRef)

	exists, err := core.encryption.keyWrapper.KeyExists(ctx, mistypedContentRef)
	require.NoError(t, err)
	require.True(t, exists, "mistyped content_key_ref must not be shredded through the DEK store")
}

func TestDeleteUser_CryptoShredEventTombstonesMessagesAndDeletesAssetGraph(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "shredauthor", "Shred Author", "password123")
	require.NoError(t, err)
	viewer, err := core.CreateUser(ctx, "system", "shredviewer", "Shred Viewer", "password123")
	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)
	_, err = core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
	require.NoError(t, err)

	original, err := core.UploadAttachment(ctx, author.Id, room.Id, "secret.txt", "text/plain", bytes.NewReader([]byte("secret original")))
	require.NoError(t, err)
	thumbnail, err := core.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(16, 16)))
	require.NoError(t, err)
	variant, err := core.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT, room.Id, "variant.mp4", "video/mp4", bytes.NewReader([]byte("secret variant")))
	require.NoError(t, err)
	avatar, err := core.UploadUserAvatar(ctx, author.Id, bytes.NewReader(createTestPNG(64, 64)))
	require.NoError(t, err)
	require.NoError(t, core.SetUserAvatar(ctx, author.Id, avatar))

	event, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "message with secret asset", []string{original.Id}, "", "", nil, false)
	require.NoError(t, err)

	for _, att := range []*corev1.Attachment{original, thumbnail, variant} {
		_, _, err := core.GetAttachmentReader(ctx, att)
		require.NoError(t, err, "precondition: %s should be readable before shred", att.Id)
	}

	require.NoError(t, core.DeleteUser(ctx, author.Id, author.Id))

	shredEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(author.Id).Subject(events.EventUserKeyShredded))
	require.NoError(t, err)
	require.Len(t, shredEvents, 1)
	require.Equal(t, author.Id, shredEvents[0].GetActorId())
	require.Equal(t, author.Id, shredEvents[0].GetUserKeyShredded().GetUserId())

	fullBody, err := core.GetFullMessageBodyByEventID(ctx, event.Id)
	require.NoError(t, err)
	require.Nil(t, fullBody, "message body should be tombstoned by UserKeyShreddedEvent before decrypt")

	deletedIDs := map[string]bool{}
	for _, att := range []*corev1.Attachment{original, thumbnail, variant} {
		deletedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(att.Id).Subject(events.EventAssetDeleted))
		require.NoError(t, err)
		require.Len(t, deletedEvents, 1)
		e := deletedEvents[0]
		deletedIDs[e.GetAssetDeleted().GetAssetId()] = true
	}
	require.True(t, deletedIDs[original.Id], "source asset should get AssetDeletedEvent")
	require.True(t, deletedIDs[thumbnail.Id], "thumbnail derivative should get AssetDeletedEvent")
	require.True(t, deletedIDs[variant.Id], "variant derivative should get AssetDeletedEvent")

	for _, att := range []*corev1.Attachment{original, thumbnail, variant} {
		got, err := core.LookupAttachment(ctx, signedurl.AttachmentLocator{
			RoomID:       room.Id,
			AttachmentID: att.Id,
			UserID:       viewer.Id,
			ExpiresAt:    time.Now().Add(time.Minute).Unix(),
		})
		require.NoError(t, err)
		require.Nil(t, got, "deleted asset %s should no longer resolve through the serving projection", att.Id)

		_, _, err = core.GetAttachmentReader(ctx, att)
		require.Error(t, err, "backing bytes for %s should be deleted", att.Id)
	}

	userAssetDeletedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(author.Id).Subject(events.EventAssetDeleted))
	require.NoError(t, err)
	require.Len(t, userAssetDeletedEvents, 1)
	require.Equal(t, avatar.Id, userAssetDeletedEvents[0].GetAssetDeleted().GetAssetId())
	deletedAvatar, err := core.GetUserAvatar(ctx, author.Id)
	require.NoError(t, err)
	require.Nil(t, deletedAvatar)
	_, err = core.storage.serverAssets.Get(ctx, avatar.Id)
	require.Error(t, err, "avatar backing bytes should be deleted after key shred and account deletion")
}

func TestEditMessage_PreservesEncryptionState(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// Post an encrypted message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Original content", nil, "", "", nil, false)
	require.NoError(t, err)
	messageBodyID := event.Id

	// Edit the message
	err = core.EditMessage(ctx, user.Id, KindChannel, room.Id, messageBodyID, "Edited content")
	require.NoError(t, err)

	// Post-#597 cutover: the edited body rides on a MessageEditedEvent
	// in the EVT stream, surfaced via the projection's LatestBody.
	stored, retracted, ok := core.RoomTimeline.LatestBody(event.Id)
	require.True(t, ok, "expected the edited message to still be projected")
	require.False(t, retracted, "message should not be retracted by an edit")
	require.NotNil(t, stored, "expected a body after edit")
	require.NotEmpty(t, stored.EncryptedBody, "encrypted body should not be empty after edit")
	require.NotEmpty(t, stored.EncryptionNonce, "nonce should not be empty after edit")
	require.Equal(t, encryption.EnvelopeVersionV2, stored.EncryptionVersion, "edited messages should keep v2 envelopes")
	require.EqualValues(t, 1, stored.ContentKeyEpoch, "edited messages should reference the active message-body DEK epoch")

	// Verify we can read the edited content
	body, err := core.GetMessageBody(ctx, KindChannel, messageBodyID)
	require.NoError(t, err)
	require.Equal(t, "Edited content", body)
}

func TestCrossUserDecryption(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Create two users
	userA, err := core.CreateUser(ctx, "system", "userA", "User A", "password123")
	require.NoError(t, err)

	userB, err := core.CreateUser(ctx, "system", "userB", "User B", "password123")
	require.NoError(t, err)

	// User A creates a space and room
	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, userA.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// User A posts a message
	eventA, err := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Message from User A", nil, "", "", nil, false)
	require.NoError(t, err)

	// User B should be able to read User A's message (decrypted)
	bodyA, err := core.GetMessageBody(ctx, KindChannel, eventA.Id)
	require.NoError(t, err)
	require.Equal(t, "Message from User A", bodyA, "User B should be able to read User A's decrypted message")

	// User B posts a message
	eventB, err := core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Message from User B", nil, "", "", nil, false)
	require.NoError(t, err)

	// User A should be able to read User B's message (decrypted)
	bodyB, err := core.GetMessageBody(ctx, KindChannel, eventB.Id)
	require.NoError(t, err)
	require.Equal(t, "Message from User B", bodyB, "User A should be able to read User B's decrypted message")
}
