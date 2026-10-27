package connectapi

import (
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAccountActionAvailability(t *testing.T) {
	env := newConnectAPITestEnv(t)
	lastChange := time.Now().Add(-time.Hour).UTC()
	actions, err := accountActions(env.ctx, env.api, env.viewer, false, false, lastChange)
	require.NoError(t, err)
	require.False(t, actions.ChangeUsername.Allowed)
	require.Equal(t, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_COOLDOWN, actions.ChangeUsername.Reason)
	require.Equal(t, lastChange.Add(core.LoginChangeCooldown), actions.ChangeUsername.AvailableAt.AsTime())
	require.True(t, actions.ChangePassword.Allowed)
	require.Equal(t, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_PERMISSION_DENIED, actions.DeleteAccount.Reason)

	// Management takes precedence even when the caller can bypass cooldowns.
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageAccounts))
	env.api.config.Auth.DirectLogin = new(false)
	actions, err = accountActions(env.ctx, env.api, env.viewer, true, true, lastChange)
	require.NoError(t, err)
	require.Equal(t, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_EXTERNALLY_MANAGED, actions.ChangeUsername.Reason)
	require.Nil(t, actions.ChangeUsername.AvailableAt)
	require.False(t, actions.ChangePassword.Allowed)
	require.Equal(t, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_DISABLED_BY_OPERATOR, actions.ChangePassword.Reason)
	require.True(t, actions.DeleteAccount.Allowed)
	require.Equal(t, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_UNSPECIFIED, actions.DeleteAccount.Reason)

	actions, err = accountActions(env.ctx, env.api, env.viewer, false, true, lastChange)
	require.NoError(t, err)
	require.True(t, actions.ChangeUsername.Allowed)
	require.Nil(t, actions.ChangeUsername.AvailableAt)

	ordinary, err := env.core.CreateUser(env.ctx, core.SystemActorID, "availability-user", "Person", "password123")
	require.NoError(t, err)
	actions, err = accountActions(env.ctx, env.api, ordinary, false, true, time.Now().Add(-core.LoginChangeCooldown-time.Hour))
	require.NoError(t, err)
	require.True(t, actions.ChangeUsername.Allowed)
	require.Nil(t, actions.ChangeUsername.AvailableAt)
}
