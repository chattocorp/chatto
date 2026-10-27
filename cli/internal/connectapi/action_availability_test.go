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
	require.Equal(t, apiv1.UsernameChangeAvailability_STATE_ALLOWED, actions.ChangeUsername.State)
	require.Equal(t, lastChange.Add(core.LoginChangeCooldown), actions.ChangeUsername.AvailableAt.AsTime())
	require.Equal(t, apiv1.PasswordChangeAvailability_PASSWORD_CHANGE_AVAILABILITY_ALLOWED, actions.GetChangePassword())
	require.Equal(t, apiv1.AccountDeletionAvailability_ACCOUNT_DELETION_AVAILABILITY_PERMISSION_DENIED, actions.GetDeleteAccount())

	// Policy and timing are independent: management does not remove a cooldown.
	actions, err = accountActions(env.ctx, env.api, env.viewer, true, false, lastChange)
	require.NoError(t, err)
	require.Equal(t, apiv1.UsernameChangeAvailability_STATE_PROVIDER_MANAGED, actions.ChangeUsername.State)
	require.Equal(t, lastChange.Add(core.LoginChangeCooldown), actions.ChangeUsername.AvailableAt.AsTime())
	// Management still blocks callers who can bypass cooldowns.
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageAccounts))
	env.api.config.Auth.DirectLogin = new(false)
	actions, err = accountActions(env.ctx, env.api, env.viewer, true, true, lastChange)
	require.NoError(t, err)
	require.Equal(t, apiv1.UsernameChangeAvailability_STATE_PROVIDER_MANAGED, actions.ChangeUsername.State)
	require.Nil(t, actions.ChangeUsername.AvailableAt)
	require.Equal(t, apiv1.PasswordChangeAvailability_PASSWORD_CHANGE_AVAILABILITY_DIRECT_LOGIN_DISABLED, actions.GetChangePassword())
	require.Equal(t, apiv1.AccountDeletionAvailability_ACCOUNT_DELETION_AVAILABILITY_ALLOWED, actions.GetDeleteAccount())

	actions, err = accountActions(env.ctx, env.api, env.viewer, false, true, lastChange)
	require.NoError(t, err)
	require.Equal(t, apiv1.UsernameChangeAvailability_STATE_ALLOWED, actions.ChangeUsername.State)
	require.Nil(t, actions.ChangeUsername.AvailableAt)

	ordinary, err := env.core.CreateUser(env.ctx, core.SystemActorID, "availability-user", "Person", "password123")
	require.NoError(t, err)
	actions, err = accountActions(env.ctx, env.api, ordinary, false, true, time.Now().Add(-core.LoginChangeCooldown-time.Hour))
	require.NoError(t, err)
	require.Equal(t, apiv1.UsernameChangeAvailability_STATE_ALLOWED, actions.ChangeUsername.State)
	require.Nil(t, actions.ChangeUsername.AvailableAt)
}
