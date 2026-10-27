/** Resource-scoped action snapshots. Commands remain authoritative. */
import {
  ActionUnavailableReason,
  type ActionAvailability as APIActionAvailability
} from '@chatto/api-types/api/v1/action_availability_pb';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';

export { ActionUnavailableReason };

export type ActionAvailability = {
  allowed: boolean;
  /** Preserve unknown numeric reasons; they must not enable a blocked action. */
  reason: ActionUnavailableReason;
  /** Earliest cooldown refresh time, not an automatic permission grant. */
  availableAt?: string;
};

/** Absence is unknown. Do not convert it into a denial. */
export function actionAvailabilityFromAPI(
  value?: APIActionAvailability
): ActionAvailability | undefined {
  if (!value) return undefined;
  return {
    allowed: value.allowed,
    reason: value.allowed ? ActionUnavailableReason.UNSPECIFIED : value.reason,
    availableAt:
      !value.allowed && value.reason === ActionUnavailableReason.COOLDOWN
        ? value.availableAt?.toDate().toISOString()
        : undefined
  };
}

/** Compatibility for servers that do not report username action snapshots. */
export function legacyUsernameChangeAvailability(
  lastChange: string | null | undefined,
  bypassCooldown: boolean,
  now = Date.now()
): ActionAvailability {
  const availableAt = lastChange ? Date.parse(lastChange) + 30 * 24 * 60 * 60 * 1000 : 0;
  const blocked = !bypassCooldown && availableAt > now;
  return {
    allowed: !blocked,
    reason: blocked ? ActionUnavailableReason.COOLDOWN : ActionUnavailableReason.UNSPECIFIED,
    availableAt: blocked ? new Date(availableAt).toISOString() : undefined
  };
}

/** Compatibility for identity lists without the shared availability message. */
export function legacyDisconnectAvailability(
  eligibility: DisconnectEligibility | undefined,
  passwordUsable: boolean,
  identityCount: number
): ActionAvailability {
  switch (eligibility) {
    case DisconnectEligibility.DISABLED_BY_OPERATOR:
      return { allowed: false, reason: ActionUnavailableReason.DISABLED_BY_OPERATOR };
    case DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD:
      return {
        allowed: passwordUsable,
        reason: passwordUsable
          ? ActionUnavailableReason.UNSPECIFIED
          : ActionUnavailableReason.LAST_SIGN_IN_METHOD
      };
    case DisconnectEligibility.UNSPECIFIED:
    case undefined:
      return {
        allowed: passwordUsable || identityCount > 1,
        reason:
          passwordUsable || identityCount > 1
            ? ActionUnavailableReason.UNSPECIFIED
            : ActionUnavailableReason.LAST_SIGN_IN_METHOD
      };
    default:
      // Existing clients let the server explain unknown legacy hints on attempt.
      return { allowed: true, reason: ActionUnavailableReason.UNSPECIFIED };
  }
}
