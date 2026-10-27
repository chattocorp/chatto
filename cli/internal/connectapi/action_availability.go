package connectapi

import (
	"context"
	"time"

	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// accountActions assembles input-independent UI guidance from the same domain
// queries used by account commands. It never replaces their guarded checks.
func accountActions(ctx context.Context, api *API, user *readmodel.User, managed, canDelete bool, lastChange time.Time) (*apiv1.AccountActions, error) {
	username := &apiv1.UsernameChangeAvailability{State: apiv1.UsernameChangeAvailability_STATE_ALLOWED}
	if managed {
		username.State = apiv1.UsernameChangeAvailability_STATE_PROVIDER_MANAGED
	}
	if !lastChange.IsZero() && time.Now().Before(lastChange.Add(core.LoginChangeCooldown)) {
		bypass, err := api.core.CanManageUserAccounts(ctx, user.Id)
		if err != nil {
			return nil, err
		}
		if !bypass {
			username.AvailableAt = timestamppb.New(lastChange.Add(core.LoginChangeCooldown))
		}
	}
	password := apiv1.PasswordChangeAvailability_PASSWORD_CHANGE_AVAILABILITY_ALLOWED
	if !api.config.Auth.DirectLoginOrDefault() {
		password = apiv1.PasswordChangeAvailability_PASSWORD_CHANGE_AVAILABILITY_DIRECT_LOGIN_DISABLED
	}
	deletion := apiv1.AccountDeletionAvailability_ACCOUNT_DELETION_AVAILABILITY_ALLOWED
	if !canDelete {
		deletion = apiv1.AccountDeletionAvailability_ACCOUNT_DELETION_AVAILABILITY_PERMISSION_DENIED
	}
	if user.IsBot {
		password = apiv1.PasswordChangeAvailability_PASSWORD_CHANGE_AVAILABILITY_UNSUPPORTED_ACCOUNT
		deletion = apiv1.AccountDeletionAvailability_ACCOUNT_DELETION_AVAILABILITY_UNSUPPORTED_ACCOUNT
	}
	return &apiv1.AccountActions{ChangeUsername: username, ChangePassword: password.Enum(), DeleteAccount: deletion.Enum()}, nil
}

// disconnectAvailability gives configured restrictions precedence over the
// last-method check. Each listed identity has its own resource-scoped decision.
func disconnectAvailability(api *API, identity core.ExternalIdentity, identities []core.ExternalIdentity, hasPassword bool) apiv1.DisconnectEligibility {
	if api.core.IdentityUnlinkDisabled(identity) {
		return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_DISABLED_BY_OPERATOR
	}
	if !api.externalIdentitySignInPolicy().CanDisconnect(identity.SubjectHash, identities, hasPassword) {
		return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_LAST_ENABLED_SIGN_IN_METHOD
	}
	return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_ALLOWED
}
