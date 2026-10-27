package connectapi

import (
	"context"
	"time"

	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// actionAvailability keeps allowed snapshots free of stale denial metadata.
func actionAvailability(allowed bool, reason apiv1.ActionUnavailableReason) *apiv1.ActionAvailability {
	if allowed {
		reason = apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_UNSPECIFIED
	}
	return &apiv1.ActionAvailability{Allowed: allowed, Reason: reason}
}

// accountActions assembles input-independent UI guidance from the same domain
// queries used by account commands. It never replaces their guarded checks.
func accountActions(ctx context.Context, api *API, user *readmodel.User, managed, canDelete bool, lastChange time.Time) (*apiv1.AccountActions, error) {
	username := actionAvailability(!managed, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_EXTERNALLY_MANAGED)
	if !managed && !lastChange.IsZero() && time.Now().Before(lastChange.Add(core.LoginChangeCooldown)) {
		bypass, err := api.core.CanManageUserAccounts(ctx, user.Id)
		if err != nil {
			return nil, err
		}
		if !bypass {
			username = actionAvailability(false, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_COOLDOWN)
			username.AvailableAt = timestamppb.New(lastChange.Add(core.LoginChangeCooldown))
		}
	}
	password := actionAvailability(api.config.Auth.DirectLoginOrDefault(), apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_DISABLED_BY_OPERATOR)
	deletion := actionAvailability(canDelete, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_PERMISSION_DENIED)
	if user.IsBot {
		password = actionAvailability(false, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_UNSUPPORTED_ACCOUNT)
		deletion = actionAvailability(false, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_UNSUPPORTED_ACCOUNT)
	}
	return &apiv1.AccountActions{ChangeUsername: username, ChangePassword: password, DeleteAccount: deletion}, nil
}

// disconnectAvailability gives configured restrictions precedence over the
// last-method check. Each listed identity has its own resource-scoped decision.
func disconnectAvailability(api *API, identity core.ExternalIdentity, identities []core.ExternalIdentity, hasPassword bool) *apiv1.ActionAvailability {
	if api.core.IdentityUnlinkDisabled(identity) {
		return actionAvailability(false, apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_DISABLED_BY_OPERATOR)
	}
	return actionAvailability(api.externalIdentitySignInPolicy().CanDisconnect(identity.SubjectHash, identities, hasPassword), apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_LAST_SIGN_IN_METHOD)
}

// legacyDisconnectEligibility serves older clients from the canonical decision.
func legacyDisconnectEligibility(availability *apiv1.ActionAvailability) apiv1.DisconnectEligibility {
	if availability.Allowed {
		return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_ALLOWED
	}
	if availability.Reason == apiv1.ActionUnavailableReason_ACTION_UNAVAILABLE_REASON_DISABLED_BY_OPERATOR {
		return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_DISABLED_BY_OPERATOR
	}
	return apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_LAST_ENABLED_SIGN_IN_METHOD
}
