package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestPrivatePresencePreferenceAPI(t *testing.T) {
	env := newConnectAPITestEnv(t)
	s := &accountService{api: env.api}
	ctx := withCaller(t.Context(), env.viewer)
	read, err := s.GetPresencePreference(ctx, connect.NewRequest(&apiv1.GetPresencePreferenceRequest{}))
	require.NoError(t, err)
	require.Nil(t, read.Msg.Preference)
	saved, err := s.SetPresencePreference(ctx, connect.NewRequest(&apiv1.SetPresencePreferenceRequest{Status: apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE}))
	require.NoError(t, err)
	require.NotEmpty(t, saved.Msg.Preference.Revision)
	refresh, err := s.RefreshPresence(ctx, connect.NewRequest(&apiv1.RefreshPresenceRequest{}))
	require.NoError(t, err)
	require.Equal(t, apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE, refresh.Msg.Preference.Status)
	legacy, err := s.SetPresence(ctx, connect.NewRequest(&apiv1.SetPresenceRequest{Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE, UserSelected: true}))
	require.NoError(t, err)
	require.Equal(t, apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE, legacy.Msg.Status)
	_, err = s.SetPresencePreference(ctx, connect.NewRequest(&apiv1.SetPresencePreferenceRequest{Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE}))
	requireConnectCode(t, err, connect.CodeAborted)
	_, err = s.GetPresencePreference(t.Context(), connect.NewRequest(&apiv1.GetPresencePreferenceRequest{}))
	requireConnectCode(t, err, connect.CodeUnauthenticated)
}
