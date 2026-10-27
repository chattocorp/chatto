/** Action-specific state enums. Only ALLOWED enables an action; commands remain authoritative. */
import {
  UsernameChangeAvailability_State as UsernameChangeState,
  type UsernameChangeAvailability as APIUsernameChangeAvailability
} from '@chatto/api-types/api/v1/account_actions_pb';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';

export { UsernameChangeState, DisconnectEligibility };
export {
  PasswordChangeAvailability,
  AccountDeletionAvailability
} from '@chatto/api-types/api/v1/account_actions_pb';
export { UsernameChoiceAvailability } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';

/** Username changes need a message because cooldowns carry a refresh timestamp. */
export type UsernameChangeAvailability = {
  state: UsernameChangeState;
  /** Earliest cooldown refresh time, not an automatic permission grant. */
  availableAt?: string;
};

/** Preserve absence and unknown states. Timing belongs only to COOLDOWN. */
export function usernameChangeAvailabilityFromAPI(
  value?: APIUsernameChangeAvailability
): UsernameChangeAvailability | undefined {
  if (!value) return undefined;
  return {
    state: value.state,
    availableAt:
      value.state === UsernameChangeState.COOLDOWN
        ? value.availableAt?.toDate().toISOString()
        : undefined
  };
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
    state: blocked ? UsernameChangeState.COOLDOWN : UsernameChangeState.ALLOWED,
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
