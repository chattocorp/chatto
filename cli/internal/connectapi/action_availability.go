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
	username := &apiv1.UsernameChangeAvailability{Allowed: !managed}
	if managed {
		username.Reason = apiv1.UsernameChangeAvailability_REASON_PROVIDER_MANAGED
	}
	if !managed && !lastChange.IsZero() && time.Now().Before(lastChange.Add(core.LoginChangeCooldown)) {
		bypass, err := api.core.CanManageUserAccounts(ctx, user.Id)
		if err != nil {
			return nil, err
		}
		if !bypass {
			username.Allowed = false
			username.Reason = apiv1.UsernameChangeAvailability_REASON_COOLDOWN
			username.AvailableAt = timestamppb.New(lastChange.Add(core.LoginChangeCooldown))
		}
	}
	password := &apiv1.PasswordChangeAvailability{Allowed: api.config.Auth.DirectLoginOrDefault()}
	if !password.Allowed {
		password.Reason = apiv1.PasswordChangeAvailability_REASON_DIRECT_LOGIN_DISABLED
	}
	deletion := &apiv1.AccountDeletionAvailability{Allowed: canDelete}
	if !deletion.Allowed {
		deletion.Reason = apiv1.AccountDeletionAvailability_REASON_PERMISSION_DENIED
	}
	if user.IsBot {
		password = &apiv1.PasswordChangeAvailability{Reason: apiv1.PasswordChangeAvailability_REASON_UNSUPPORTED_ACCOUNT}
		deletion = &apiv1.AccountDeletionAvailability{Reason: apiv1.AccountDeletionAvailability_REASON_UNSUPPORTED_ACCOUNT}
	}
	return &apiv1.AccountActions{ChangeUsername: username, ChangePassword: password, DeleteAccount: deletion}, nil
}

// disconnectAvailability gives configured restrictions precedence over the
// last-method check. Each listed identity has its own resource-scoped decision.
func disconnectAvailability(api *API, identity core.ExternalIdentity, identities []core.ExternalIdentity, hasPassword bool) *apiv1.IdentityDisconnectAvailability {
	if api.core.IdentityUnlinkDisabled(identity) {
		return &apiv1.IdentityDisconnectAvailability{Reason: apiv1.IdentityDisconnectAvailability_REASON_DISABLED_BY_OPERATOR}
	}
	if !api.externalIdentitySignInPolicy().CanDisconnect(identity.SubjectHash, identities, hasPassword) {
		return &apiv1.IdentityDisconnectAvailability{Reason: apiv1.IdentityDisconnectAvailability_REASON_LAST_SIGN_IN_METHOD}
	}
	return &apiv1.IdentityDisconnectAvailability{Allowed: true}
}

// legacyDisconnectEligibility serves older clients from the canonical decision.
func legacyDisconnectEligibility(availability *apiv1.IdentityDisconnectAvailability) apiv1.DisconnectEligibility {
	if availability.Allowed {
		return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_ALLOWED
	}
	if availability.Reason == apiv1.IdentityDisconnectAvailability_REASON_DISABLED_BY_OPERATOR {
		return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_DISABLED_BY_OPERATOR
	}
	return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_LAST_ENABLED_SIGN_IN_METHOD
}
