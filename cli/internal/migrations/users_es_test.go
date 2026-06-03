package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

type corruptingWrapKeyWrapper struct {
	kms.KeyWrapper
}

func (w corruptingWrapKeyWrapper) WrapContentKey(ctx context.Context, keyRef string, contentKey, aad []byte) (*kms.WrappedContentKey, error) {
	wrongKey, err := encryption.GenerateKey()
	if err != nil {
		return nil, err
	}
	return w.KeyWrapper.WrapContentKey(ctx, keyRef, wrongKey, aad)
}

func TestMigrateUsersToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)
	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))
}

func TestMigrateUsersToES_SeedsEncryptedUserAggregateAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	verifiedAt := createdAt.Add(time.Hour)
	loginChangedAt := createdAt.Add(2 * time.Hour)
	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(createdAt),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)
	_, err := kv.Put(ctx, "auth.U1.password", []byte("hash"))
	require.NoError(t, err)
	putProtoKV(t, ctx, kv, "user.U1.avatar", &corev1.DeprecatedAsset{
		Asset: &corev1.DeprecatedAsset_S3{S3: &corev1.S3Asset{Key: "avatars/U1"}},
	})
	putProtoKV(t, ctx, kv, "verified_emails.U1.emailhash", &corev1.VerifiedEmail{
		Email:      "Alice@Example.com",
		VerifiedAt: timestamppb.New(verifiedAt),
	})
	tz := "Europe/Berlin"
	putProtoKV(t, ctx, kv, "user_preferences.U1", &corev1.ServerUserPreferences{
		Timezone:   proto.String(tz),
		TimeFormat: corev1.TimeFormat_TIME_FORMAT_24H,
	})
	_, err = kv.Put(ctx, "user_login_changed_at.U1", []byte(loginChangedAt.Format(time.RFC3339)))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "user_by_oidc.subjecthash", []byte("U1"))
	require.NoError(t, err)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 8, info.State.Msgs)

	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 8)
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[0].GetEvent())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserAccountCreated{}, eventsBySeq[2].GetEvent())
	require.IsType(t, &corev1.Event_UserPasswordHashChanged{}, eventsBySeq[3].GetEvent())
	require.IsType(t, &corev1.Event_UserAvatarSet{}, eventsBySeq[4].GetEvent())
	require.IsType(t, &corev1.Event_UserVerifiedEmailAdded{}, eventsBySeq[5].GetEvent())
	require.IsType(t, &corev1.Event_UserOidcSubjectLinked{}, eventsBySeq[6].GetEvent())
	require.IsType(t, &corev1.Event_UserLoginCooldownStarted{}, eventsBySeq[7].GetEvent())

	messageDEK := eventsBySeq[0].GetUserDekGenerated()
	require.Equal(t, "U1", messageDEK.GetUserId())
	require.EqualValues(t, 1, messageDEK.GetEpoch())
	require.Equal(t, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, messageDEK.GetPurpose())
	require.NotEmpty(t, messageDEK.GetContentKeyRef())
	require.NotEmpty(t, messageDEK.GetWrappingKeyRef())
	storedMessageDEK, err := contentKeys.Get(ctx, messageDEK.GetContentKeyRef())
	require.NoError(t, err)
	require.NotEmpty(t, storedMessageDEK.GetEncryptedContentKey())
	require.NotEmpty(t, storedMessageDEK.GetContentKeyNonce())
	require.Equal(t, messageDEK.GetWrappingKeyRef(), storedMessageDEK.GetWrappingKeyRef())

	piiDEKEvent := eventsBySeq[1].GetUserDekGenerated()
	require.Equal(t, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, piiDEKEvent.GetPurpose())
	unwrappedPIIDEK, err := unwrapMigrationDEK(ctx, keyWrapper, contentKeys, piiDEKEvent)
	require.NoError(t, err)

	account := eventsBySeq[2].GetUserAccountCreated()
	require.Equal(t, "U1", account.GetUserId())
	require.NotNil(t, account.GetEncryptedLogin())
	require.NotNil(t, account.GetEncryptedDisplayName())
	require.NotContains(t, string(eventsBySeq[2].GetUserAccountCreated().GetEncryptedLogin().GetEncryptedValue()), "Alice")
	login := decryptImportedUserString(t, unwrappedPIIDEK, eventsBySeq[2].GetId(), "U1", events.EventUserAccountCreated, "login", account.GetEncryptedLogin())
	require.Equal(t, "Alice", login)
	displayName := decryptImportedUserString(t, unwrappedPIIDEK, eventsBySeq[2].GetId(), "U1", events.EventUserAccountCreated, "display_name", account.GetEncryptedDisplayName())
	require.Equal(t, "Alice A.", displayName)

	email := eventsBySeq[5].GetUserVerifiedEmailAdded()
	require.NotNil(t, email.GetEncryptedEmail())
	emailPlaintext := decryptImportedUserString(t, unwrappedPIIDEK, eventsBySeq[5].GetId(), "U1", events.EventUserVerifiedEmailAdded, "email", email.GetEncryptedEmail())
	require.Equal(t, "Alice@Example.com", emailPlaintext)
	require.True(t, eventsBySeq[5].GetCreatedAt().AsTime().Equal(verifiedAt))
	require.Equal(t, "subjecthash", eventsBySeq[6].GetUserOidcSubjectLinked().GetSubjectHash())
	require.Equal(t, "U1", eventsBySeq[7].GetUserLoginCooldownStarted().GetUserId())
	require.True(t, eventsBySeq[7].GetCreatedAt().AsTime().Equal(loginChangedAt))

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 8, infoReplay.State.Msgs)
}

func TestMigrateUsersToES_VerifiesCreatedDEKBeforePublishing(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)

	err := MigrateUsersToES(ctx, kv, publisher, corruptingWrapKeyWrapper{KeyWrapper: keyWrapper}, contentKeys, testLogger())
	require.Error(t, err)
	require.Contains(t, err.Error(), "verify migration DEK")

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}

func TestMigrateUsersToES_AppendsEncryptedRepairForPlaintextUserEVTPrefix(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(createdAt),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)
	putProtoKV(t, ctx, kv, "verified_emails.U1.emailhash", &corev1.VerifiedEmail{
		Email:      "Alice@Example.com",
		VerifiedAt: timestamppb.New(createdAt.Add(time.Hour)),
	})

	legacyPlaintextAccount := stamp(&corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U1"},
	}}, "system:migration", timestamppb.New(createdAt))
	_, err := publisher.AppendBatch(ctx, []events.BatchEntry{{
		Subject:       events.UserAggregate("U1").SubjectFor(legacyPlaintextAccount),
		Event:         legacyPlaintextAccount,
		HasOCC:        true,
		ExpectedSeq:   0,
		FilterSubject: events.UserAggregate("U1").AllEventsFilter(),
	}})
	require.NoError(t, err)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, info.State.Msgs)
	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 5)
	require.Nil(t, eventsBySeq[0].GetUserAccountCreated().GetEncryptedLogin())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[2].GetEvent())
	require.IsType(t, &corev1.Event_UserAccountCreated{}, eventsBySeq[3].GetEvent())
	require.IsType(t, &corev1.Event_UserVerifiedEmailAdded{}, eventsBySeq[4].GetEvent())

	piiDEK, err := unwrapMigrationDEK(ctx, keyWrapper, contentKeys, eventsBySeq[2].GetUserDekGenerated())
	require.NoError(t, err)
	account := eventsBySeq[3].GetUserAccountCreated()
	require.Equal(t, "Alice", decryptImportedUserString(t, piiDEK, eventsBySeq[3].GetId(), "U1", events.EventUserAccountCreated, "login", account.GetEncryptedLogin()))
	email := eventsBySeq[4].GetUserVerifiedEmailAdded()
	require.Equal(t, "Alice@Example.com", decryptImportedUserString(t, piiDEK, eventsBySeq[4].GetId(), "U1", events.EventUserVerifiedEmailAdded, "email", email.GetEncryptedEmail()))

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, infoReplay.State.Msgs)
}

func TestMigrateUsersToES_ResumesAfterCommittedDEKsAndEncryptedAccount(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	verifiedAt := createdAt.Add(time.Hour)
	loginChangedAt := createdAt.Add(2 * time.Hour)
	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(createdAt),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)
	_, err := kv.Put(ctx, "auth.U1.password", []byte("hash"))
	require.NoError(t, err)
	putProtoKV(t, ctx, kv, "verified_emails.U1.emailhash", &corev1.VerifiedEmail{
		Email:      "Alice@Example.com",
		VerifiedAt: timestamppb.New(verifiedAt),
	})
	_, err = kv.Put(ctx, "user_by_oidc.subjecthash", []byte("U1"))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "user_login_changed_at.U1", []byte(loginChangedAt.Format(time.RFC3339)))
	require.NoError(t, err)

	entries, _, err := buildUserMigrationEntries(ctx, kv, keyWrapper, contentKeys, user, createdAt, []string{"subjecthash"}, nil, testLogger())
	require.NoError(t, err)
	appendUserMigrationEntries(t, ctx, publisher, "U1", entries[:3], 0)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 7, info.State.Msgs)
	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 7)
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[0].GetEvent())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserAccountCreated{}, eventsBySeq[2].GetEvent())
	require.IsType(t, &corev1.Event_UserPasswordHashChanged{}, eventsBySeq[3].GetEvent())
	require.IsType(t, &corev1.Event_UserVerifiedEmailAdded{}, eventsBySeq[4].GetEvent())
	require.IsType(t, &corev1.Event_UserOidcSubjectLinked{}, eventsBySeq[5].GetEvent())
	require.IsType(t, &corev1.Event_UserLoginCooldownStarted{}, eventsBySeq[6].GetEvent())
}

func TestMigrateUsersToES_RepairDoesNotDuplicateExistingDEK(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(createdAt),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)
	putProtoKV(t, ctx, kv, "verified_emails.U1.emailhash", &corev1.VerifiedEmail{
		Email:      "Alice@Example.com",
		VerifiedAt: timestamppb.New(createdAt.Add(time.Hour)),
	})

	entries, _, err := buildUserMigrationEntries(ctx, kv, keyWrapper, contentKeys, user, createdAt, nil, nil, testLogger())
	require.NoError(t, err)
	legacyPlaintextAccount := stamp(&corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U1"},
	}}, "system:migration", timestamppb.New(createdAt))
	appendUserMigrationEntries(t, ctx, publisher, "U1", []events.BatchEntry{
		entries[0],
		{Subject: events.UserAggregate("U1").SubjectFor(legacyPlaintextAccount), Event: legacyPlaintextAccount},
	}, 0)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, info.State.Msgs)
	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 5)
	require.Equal(t, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, eventsBySeq[0].GetUserDekGenerated().GetPurpose())
	require.Nil(t, eventsBySeq[1].GetUserAccountCreated().GetEncryptedLogin())
	require.Equal(t, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, eventsBySeq[2].GetUserDekGenerated().GetPurpose())
	require.NotNil(t, eventsBySeq[3].GetUserAccountCreated().GetEncryptedLogin())
	require.NotNil(t, eventsBySeq[4].GetUserVerifiedEmailAdded().GetEncryptedEmail())

	dekCount := 0
	for _, event := range eventsBySeq {
		if event.GetUserDekGenerated() != nil {
			dekCount++
		}
	}
	require.Equal(t, 2, dekCount)
}

func TestMigrateUsersToES_RepairResumesAfterEncryptedAccount(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(createdAt),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)
	putProtoKV(t, ctx, kv, "verified_emails.U1.emailhash", &corev1.VerifiedEmail{
		Email:      "Alice@Example.com",
		VerifiedAt: timestamppb.New(createdAt.Add(time.Hour)),
	})
	_, err := kv.Put(ctx, "user_by_oidc.subjecthash", []byte("U1"))
	require.NoError(t, err)

	entries, _, err := buildUserMigrationEntries(ctx, kv, keyWrapper, contentKeys, user, createdAt, []string{"subjecthash"}, nil, testLogger())
	require.NoError(t, err)
	legacyPlaintextAccount := stamp(&corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U1"},
	}}, "system:migration", timestamppb.New(createdAt))
	appendUserMigrationEntries(t, ctx, publisher, "U1", []events.BatchEntry{
		{Subject: events.UserAggregate("U1").SubjectFor(legacyPlaintextAccount), Event: legacyPlaintextAccount},
		entries[0],
		entries[1],
		entries[2],
	}, 0)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 6, info.State.Msgs)
	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 6)
	require.Nil(t, eventsBySeq[0].GetUserAccountCreated().GetEncryptedLogin())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[2].GetEvent())
	require.NotNil(t, eventsBySeq[3].GetUserAccountCreated().GetEncryptedLogin())
	require.NotNil(t, eventsBySeq[4].GetUserVerifiedEmailAdded().GetEncryptedEmail())
	require.Equal(t, "subjecthash", eventsBySeq[5].GetUserOidcSubjectLinked().GetSubjectHash())

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 6, infoReplay.State.Msgs)
}

func TestMigrateUsersToES_SkipsCorruptOptionalLegacyValues(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper, contentKeys := setupUserMigrationKMS(t, ctx)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	user := &corev1.User{
		Id:          "U1",
		Login:       "Alice",
		DisplayName: "Alice A.",
		CreatedAt:   timestamppb.New(createdAt),
	}
	putProtoKV(t, ctx, kv, "user.U1", user)
	_, err := kv.Put(ctx, "user.U1.avatar", []byte("not protobuf"))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "user_login_changed_at.U1", []byte("not a timestamp"))
	require.NoError(t, err)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, contentKeys, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, info.State.Msgs)
	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 3)
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[0].GetEvent())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserAccountCreated{}, eventsBySeq[2].GetEvent())
}

func TestSortLegacyEmailEvents_UsesLegacyKeyForTimestampTies(t *testing.T) {
	ts := timestamppb.New(time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC))
	items := []legacyEmailEvent{{
		legacyKey: "verified_emails.U1.b",
		event:     &corev1.Event{Id: "A", CreatedAt: ts},
	}, {
		legacyKey: "verified_emails.U1.a",
		event:     &corev1.Event{Id: "Z", CreatedAt: ts},
	}}

	sortLegacyEmailEvents(items)

	require.Equal(t, "verified_emails.U1.a", items[0].legacyKey)
	require.Equal(t, "Z", items[0].event.GetId())
	require.Equal(t, "verified_emails.U1.b", items[1].legacyKey)
	require.Equal(t, "A", items[1].event.GetId())
}

func putProtoKV(t *testing.T, ctx context.Context, kv jetstream.KeyValue, key string, msg proto.Message) {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	_, err = kv.Put(ctx, key, data)
	require.NoError(t, err)
}

func setupUserMigrationKMS(t *testing.T, ctx context.Context) (kms.KeyWrapper, *dekstore.Store) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "TEST_ENCRYPTION_KEYS",
		Storage: jetstream.MemoryStorage,
	})
	require.NoError(t, err)
	return kms.NewBuiltin(kv, testLogger()), dekstore.New(kv, testLogger())
}

func decryptImportedUserString(t *testing.T, contentKey []byte, eventID, userID, eventType, purpose string, encrypted *corev1.EncryptedUserString) string {
	t.Helper()
	plaintext, err := encryption.DecryptXChaCha20Poly1305(
		contentKey,
		encrypted.GetEncryptedValue(),
		encrypted.GetNonce(),
		migrationUserPIIAAD(eventID, userID, eventType, purpose, encrypted.GetContentKeyEpoch()),
	)
	require.NoError(t, err)
	return string(plaintext)
}

func appendUserMigrationEntries(t *testing.T, ctx context.Context, publisher *events.Publisher, userID string, entries []events.BatchEntry, expectedSeq uint64) {
	t.Helper()
	chunk := append([]events.BatchEntry(nil), entries...)
	chunk[0].HasOCC = true
	chunk[0].ExpectedSeq = expectedSeq
	chunk[0].FilterSubject = events.UserAggregate(userID).AllEventsFilter()
	_, err := publisher.AppendBatch(ctx, chunk)
	require.NoError(t, err)
}

func readUserMigrationEvents(t *testing.T, ctx context.Context, stream jetstream.Stream, count int) []*corev1.Event {
	t.Helper()
	out := make([]*corev1.Event, 0, count)
	for seq := uint64(1); seq <= uint64(count); seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)
		var event corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &event))
		out = append(out, &event)
	}
	return out
}
