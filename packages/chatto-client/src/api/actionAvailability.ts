/** Action-specific policy states with independent timing; commands remain authoritative. */
import {
  UsernameChangeAvailability_State as UsernameChangeState,
  PasswordChangeAvailability,
  AccountDeletionAvailability,
  type UsernameChangeAvailability as APIUsernameChangeAvailability
} from '@chatto/api-types/api/v1/account_actions_pb';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';

export {
  UsernameChangeState,
  DisconnectEligibility,
  PasswordChangeAvailability,
  AccountDeletionAvailability
};
export { UsernameChoiceAvailability } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';

/** Username policy and an independent earliest attempt time. */
export type UsernameChangeAvailability = {
  state: UsernameChangeState;
  /** ALLOWED becomes usable at this time; blocked states remain blocked. */
  availableAt?: string;
};

/** Preserve absence, unknown states, and the independent time constraint. */
export function usernameChangeAvailabilityFromAPI(
  value?: APIUsernameChangeAvailability
): UsernameChangeAvailability | undefined {
  if (!value) return undefined;
  return {
    state: value.state,
    availableAt: value.availableAt?.toDate().toISOString()
  };
}

/** Only ALLOWED with no pending time constraint permits an attempt.
 * Invalid timestamps fail closed. Commands still recheck authority and policy.
 */
export function canChangeUsername(action: UsernameChangeAvailability, now = Date.now()): boolean {
  return (
    action.state === UsernameChangeState.ALLOWED &&
    (action.availableAt === undefined || Date.parse(action.availableAt) <= now)
  );
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
    state: UsernameChangeState.ALLOWED,
    availableAt: blocked ? new Date(availableAt).toISOString() : undefined
  };
}

/** Older servers report UNSPECIFIED or omit the hint. Only that case uses the
 * count-based fallback; explicit denials and unknown future states stay intact.
 */
export function disconnectEligibilityWithFallback(
  eligibility: DisconnectEligibility | undefined,
  passwordUsable: boolean,
  identityCount: number
): DisconnectEligibility {
  if (eligibility !== undefined && eligibility !== DisconnectEligibility.UNSPECIFIED)
    return eligibility;
  return passwordUsable || identityCount > 1
    ? DisconnectEligibility.ALLOWED
    : DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD;
}

/** Use direct-login policy only when the server omits the password decision.
 * Explicit UNSPECIFIED and unknown future states remain unchanged.
 */
export function passwordChangeAvailabilityWithFallback(
  availability: PasswordChangeAvailability | undefined,
  directLoginEnabled: boolean
): PasswordChangeAvailability {
  return (
    availability ??
    (directLoginEnabled
      ? PasswordChangeAvailability.ALLOWED
      : PasswordChangeAvailability.DIRECT_LOGIN_DISABLED)
  );
}

/** Use the legacy viewer permission only when the server omits the deletion decision.
 * Explicit UNSPECIFIED and unknown future states remain unchanged.
 */
export function accountDeletionAvailabilityWithFallback(
  availability: AccountDeletionAvailability | undefined,
  viewerCanDeleteAccount: boolean
): AccountDeletionAvailability {
  return (
    availability ??
    (viewerCanDeleteAccount
      ? AccountDeletionAvailability.ALLOWED
      : AccountDeletionAvailability.PERMISSION_DENIED)
  );
}
