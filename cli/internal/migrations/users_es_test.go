package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestMigrateUsersToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	keyWrapper := setupUserMigrationKMS(t, ctx)
	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, testLogger()))
}

func TestMigrateUsersToES_SeedsEncryptedUserAggregateAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper := setupUserMigrationKMS(t, ctx)

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

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, testLogger()))

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
	require.NotEmpty(t, messageDEK.GetEncryptedContentKey())
	require.NotEmpty(t, messageDEK.GetContentKeyNonce())
	require.NotEmpty(t, messageDEK.GetWrappingKeyRef())

	piiDEKEvent := eventsBySeq[1].GetUserDekGenerated()
	require.Equal(t, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, piiDEKEvent.GetPurpose())
	unwrappedPIIDEK, err := unwrapMigrationDEK(ctx, keyWrapper, piiDEKEvent)
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

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 8, infoReplay.State.Msgs)
}

func TestMigrateUsersToES_AppendsEncryptedRepairForPlaintextUserEVTPrefix(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	keyWrapper := setupUserMigrationKMS(t, ctx)

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

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, info.State.Msgs)
	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 5)
	require.Nil(t, eventsBySeq[0].GetUserAccountCreated().GetEncryptedLogin())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserDekGenerated{}, eventsBySeq[2].GetEvent())
	require.IsType(t, &corev1.Event_UserAccountCreated{}, eventsBySeq[3].GetEvent())
	require.IsType(t, &corev1.Event_UserVerifiedEmailAdded{}, eventsBySeq[4].GetEvent())

	piiDEK, err := unwrapMigrationDEK(ctx, keyWrapper, eventsBySeq[2].GetUserDekGenerated())
	require.NoError(t, err)
	account := eventsBySeq[3].GetUserAccountCreated()
	require.Equal(t, "Alice", decryptImportedUserString(t, piiDEK, eventsBySeq[3].GetId(), "U1", events.EventUserAccountCreated, "login", account.GetEncryptedLogin()))
	email := eventsBySeq[4].GetUserVerifiedEmailAdded()
	require.Equal(t, "Alice@Example.com", decryptImportedUserString(t, piiDEK, eventsBySeq[4].GetId(), "U1", events.EventUserVerifiedEmailAdded, "email", email.GetEncryptedEmail()))

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, keyWrapper, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, infoReplay.State.Msgs)
}

func putProtoKV(t *testing.T, ctx context.Context, kv jetstream.KeyValue, key string, msg proto.Message) {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	_, err = kv.Put(ctx, key, data)
	require.NoError(t, err)
}

func setupUserMigrationKMS(t *testing.T, ctx context.Context) kms.KeyWrapper {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "TEST_ENCRYPTION_KEYS",
		Storage: jetstream.MemoryStorage,
	})
	require.NoError(t, err)
	return kms.NewBuiltin(kv, testLogger())
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
