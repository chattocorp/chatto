package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateUsersToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, testLogger()))
}

func TestMigrateUsersToES_IgnoresLegacyUsers(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	verifiedAt := createdAt.Add(time.Hour)
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
	_, err = kv.Put(ctx, "user_login_changed_at.U1", []byte(createdAt.Add(2*time.Hour).Format(time.RFC3339)))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "user_by_oidc.subjecthash", []byte("U1"))
	require.NoError(t, err)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)

	require.NoError(t, MigrateUsersToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, infoReplay.State.Msgs)
}

func putProtoKV(t *testing.T, ctx context.Context, kv jetstream.KeyValue, key string, msg proto.Message) {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	_, err = kv.Put(ctx, key, data)
	require.NoError(t, err)
}
