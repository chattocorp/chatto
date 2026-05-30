package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateServerBrandingToES_SeedsAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	logo := &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "logo-asset"}}}
	banner := &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_S3{S3: &corev1.S3Asset{Key: "banner-asset"}}}
	putProtoKV(t, ctx, kv, "config.instance", &configv1.ServerConfig{ServerName: "Legacy"})
	putProtoKV(t, ctx, kv, "instance.logo", logo)
	putProtoKV(t, ctx, kv, "instance.banner", banner)

	// Existing non-branding config on the same subject must not block importing
	// the branding paths.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))
	require.NoError(t, MigrateServerBrandingToES(ctx, kv, publisher, testLogger()))

	subject := events.ConfigAggregate().Subject(events.EventServerLogoSet)
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.NotZero(t, msg.Sequence)

	gotValues := map[string]*corev1.DeprecatedAsset{}
	for seq := uint64(1); seq <= msg.Sequence+5; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			continue
		}
		var got corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &got))
		switch change := got.GetEvent().(type) {
		case *corev1.Event_ServerLogoSet:
			gotValues["server.logo"] = change.ServerLogoSet.GetAsset()
		case *corev1.Event_ServerBannerSet:
			gotValues["server.banner"] = change.ServerBannerSet.GetAsset()
		}
	}
	require.True(t, proto.Equal(logo, gotValues["server.logo"]))
	require.True(t, proto.Equal(banner, gotValues["server.banner"]))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, len(gotValues))
	msgsAfterFirstRun := info.State.Msgs

	require.NoError(t, MigrateServerBrandingToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, msgsAfterFirstRun, infoReplay.State.Msgs)
}

func TestMigrateServerBrandingToES_NoLegacyState(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	require.NoError(t, MigrateServerBrandingToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}
