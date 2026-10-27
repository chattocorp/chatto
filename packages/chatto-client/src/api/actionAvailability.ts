/** Action-specific snapshots. Commands remain authoritative; reasons do not cross actions. */
import {
  UsernameChangeAvailability_Reason as UsernameChangeReason,
  PasswordChangeAvailability_Reason as PasswordChangeReason,
  AccountDeletionAvailability_Reason as AccountDeletionReason,
  type UsernameChangeAvailability as APIUsernameChangeAvailability,
  type PasswordChangeAvailability as APIPasswordChangeAvailability,
  type AccountDeletionAvailability as APIAccountDeletionAvailability
} from '@chatto/api-types/api/v1/account_actions_pb';
import {
  DisconnectEligibility,
  IdentityDisconnectAvailability_Reason as IdentityDisconnectReason,
  type IdentityDisconnectAvailability as APIIdentityDisconnectAvailability
} from '@chatto/api-types/api/v1/external_identities_pb';
import {
  UsernameChoiceAvailability_Reason as UsernameChoiceReason,
  type UsernameChoiceAvailability as APIUsernameChoiceAvailability
} from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';

export {
  UsernameChangeReason,
  PasswordChangeReason,
  AccountDeletionReason,
  IdentityDisconnectReason,
  UsernameChoiceReason
};

/** Structural mapping only: each public action supplies its own reason enum. */
type Decision<R extends number> = { allowed: boolean; reason: R };

/** Self-service rename decision, including its optional cooldown refresh time. */
export type UsernameChangeAvailability = Decision<UsernameChangeReason> & {
  /** Earliest cooldown refresh time, not an automatic permission grant. */
  availableAt?: string;
};
/** Decision for setting an initial password or changing an existing password. */
export type PasswordChangeAvailability = Decision<PasswordChangeReason>;
/** Decision for starting self-service account deletion. */
export type AccountDeletionAvailability = Decision<AccountDeletionReason>;
/** Decision scoped to one linked identity, not to all of the account's providers. */
export type IdentityDisconnectAvailability = Decision<IdentityDisconnectReason>;
/** Decision for choosing a username before account creation, without a cooldown. */
export type UsernameChoiceAvailability = Decision<UsernameChoiceReason>;

/** Preserve absence and unknown denied reasons; clear stale metadata when allowed. */
function decisionFromAPI<R extends number>(
  value: Decision<R> | undefined,
  unspecified: R
): Decision<R> | undefined {
  if (!value) return undefined;
  return { allowed: value.allowed, reason: value.allowed ? unspecified : value.reason };
}

/** Username cooldown metadata is meaningful only for a blocked cooldown. */
export function usernameChangeAvailabilityFromAPI(
  value?: APIUsernameChangeAvailability
): UsernameChangeAvailability | undefined {
  const decision = decisionFromAPI(value, UsernameChangeReason.UNSPECIFIED);
  if (!decision) return undefined;
  return {
    ...decision,
    availableAt:
      !decision.allowed && decision.reason === UsernameChangeReason.COOLDOWN
        ? value?.availableAt?.toDate().toISOString()
        : undefined
  };
}

/** Map password self-service without introducing reasons from other actions. */
export function passwordChangeAvailabilityFromAPI(
  value?: APIPasswordChangeAvailability
): PasswordChangeAvailability | undefined {
  return decisionFromAPI(value, PasswordChangeReason.UNSPECIFIED);
}

/** Map self-service deletion; absence remains unknown. */
export function accountDeletionAvailabilityFromAPI(
  value?: APIAccountDeletionAvailability
): AccountDeletionAvailability | undefined {
  return decisionFromAPI(value, AccountDeletionReason.UNSPECIFIED);
}

/** Map one identity's disconnect decision; unknown reasons remain blocked. */
export function identityDisconnectAvailabilityFromAPI(
  value?: APIIdentityDisconnectAvailability
): IdentityDisconnectAvailability | undefined {
  return decisionFromAPI(value, IdentityDisconnectReason.UNSPECIFIED);
}

/** Map username choice independently of post-creation username changes. */
export function usernameChoiceAvailabilityFromAPI(
  value?: APIUsernameChoiceAvailability
): UsernameChoiceAvailability | undefined {
  return decisionFromAPI(value, UsernameChoiceReason.UNSPECIFIED);
}

/** Compatibility for servers that do not report username action snapshots. */
export function legacyUsernameChangeAvailability(
  lastChange: string | null | undefined,
  bypassCooldown: boolean,
  now = Date.now()
): UsernameChangeAvailability {
  const availableAt = lastChange ? Date.parse(lastChange) + 30 * 24 * 60 * 60 * 1000 : 0;
  const blocked = !bypassCooldown && availableAt > now;
  return {
    allowed: !blocked,
    reason: blocked ? UsernameChangeReason.COOLDOWN : UsernameChangeReason.UNSPECIFIED,
    availableAt: blocked ? new Date(availableAt).toISOString() : undefined
  };
}

/** Compatibility for identity lists without a disconnect decision. */
export function legacyDisconnectAvailability(
  eligibility: DisconnectEligibility | undefined,
  passwordUsable: boolean,
  identityCount: number
): IdentityDisconnectAvailability {
  switch (eligibility) {
    case DisconnectEligibility.DISABLED_BY_OPERATOR:
      return { allowed: false, reason: IdentityDisconnectReason.DISABLED_BY_OPERATOR };
    case DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD:
      return {
        allowed: passwordUsable,
        reason: passwordUsable
          ? IdentityDisconnectReason.UNSPECIFIED
          : IdentityDisconnectReason.LAST_SIGN_IN_METHOD
      };
    case DisconnectEligibility.UNSPECIFIED:
    case undefined:
      return {
        allowed: passwordUsable || identityCount > 1,
        reason:
          passwordUsable || identityCount > 1
            ? IdentityDisconnectReason.UNSPECIFIED
            : IdentityDisconnectReason.LAST_SIGN_IN_METHOD
      };
    default:
      // Existing clients let the server explain unknown legacy hints on attempt.
      return { allowed: true, reason: IdentityDisconnectReason.UNSPECIFIED };
  }
}
