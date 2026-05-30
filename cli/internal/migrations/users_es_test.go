package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateUsersToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, testLogger()))
}

func TestMigrateUsersToES_SeedsUserAggregateAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

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

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 6, info.State.Msgs)

	eventsBySeq := readUserMigrationEvents(t, ctx, stream, 6)
	require.IsType(t, &corev1.Event_UserAccountCreated{}, eventsBySeq[0].GetEvent())
	require.IsType(t, &corev1.Event_UserPasswordHashChanged{}, eventsBySeq[1].GetEvent())
	require.IsType(t, &corev1.Event_UserAvatarSet{}, eventsBySeq[2].GetEvent())
	require.IsType(t, &corev1.Event_UserVerifiedEmailAdded{}, eventsBySeq[3].GetEvent())
	require.IsType(t, &corev1.Event_UserOidcSubjectLinked{}, eventsBySeq[4].GetEvent())
	require.IsType(t, &corev1.Event_UserLoginCooldownStarted{}, eventsBySeq[5].GetEvent())

	require.Equal(t, "U1", eventsBySeq[0].GetUserAccountCreated().GetUserId())
	require.Equal(t, "Alice", eventsBySeq[0].GetUserAccountCreated().GetLogin())
	require.Equal(t, "Alice A.", eventsBySeq[0].GetUserAccountCreated().GetDisplayName())
	require.Equal(t, "Alice@Example.com", eventsBySeq[3].GetUserVerifiedEmailAdded().GetEmail())
	require.True(t, eventsBySeq[3].GetCreatedAt().AsTime().Equal(verifiedAt))
	require.Equal(t, "subjecthash", eventsBySeq[4].GetUserOidcSubjectLinked().GetSubjectHash())
	require.Equal(t, "U1", eventsBySeq[5].GetUserLoginCooldownStarted().GetUserId())
	require.True(t, eventsBySeq[5].GetCreatedAt().AsTime().Equal(loginChangedAt))

	msg, err := stream.GetLastMsgForSubject(ctx, events.UserAggregate("U1").AllEventsFilter())
	require.NoError(t, err)
	require.EqualValues(t, 6, msg.Sequence)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 6, infoReplay.State.Msgs)
}

func putProtoKV(t *testing.T, ctx context.Context, kv jetstream.KeyValue, key string, msg proto.Message) {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	_, err = kv.Put(ctx, key, data)
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
