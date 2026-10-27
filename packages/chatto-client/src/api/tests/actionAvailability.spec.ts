import { describe, expect, it } from 'vitest';
import { Timestamp } from '@bufbuild/protobuf';
import { ActionAvailability as APIActionAvailability } from '@chatto/api-types/api/v1/action_availability_pb';
import {
  ActionUnavailableReason,
  actionAvailabilityFromAPI,
  legacyUsernameChangeAvailability,
  legacyDisconnectAvailability
} from '../actionAvailability.js';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';

describe('action availability', () => {
  it('keeps absence distinct from a denied action', () => {
    expect(actionAvailabilityFromAPI()).toBeUndefined();
    expect(actionAvailabilityFromAPI(new APIActionAvailability())?.allowed).toBe(false);
  });

  it('preserves unknown blocking reasons without enabling the action', () => {
    expect(
      actionAvailabilityFromAPI(
        new APIActionAvailability({ allowed: false, reason: 99 as ActionUnavailableReason })
      )
    ).toMatchObject({ allowed: false, reason: 99 });
  });

  it('does not turn an expired cooldown snapshot into permission', () => {
    expect(
      actionAvailabilityFromAPI(
        new APIActionAvailability({
          allowed: false,
          reason: ActionUnavailableReason.COOLDOWN,
          availableAt: Timestamp.fromDate(new Date('2020-01-01T00:00:00Z'))
        })
      )
    ).toMatchObject({ allowed: false, availableAt: '2020-01-01T00:00:00.000Z' });
  });

  it('discards denial metadata from allowed snapshots', () => {
    expect(
      actionAvailabilityFromAPI(
        new APIActionAvailability({
          allowed: true,
          reason: ActionUnavailableReason.COOLDOWN,
          availableAt: Timestamp.now()
        })
      )
    ).toEqual({
      allowed: true,
      reason: ActionUnavailableReason.UNSPECIFIED,
      availableAt: undefined
    });
  });

  it('retains older-server username cooldown and permission bypass behavior', () => {
    const now = Date.parse('2026-01-02T00:00:00Z');
    expect(legacyUsernameChangeAvailability('2026-01-01T00:00:00Z', false, now).allowed).toBe(
      false
    );
    expect(legacyUsernameChangeAvailability('2026-01-01T00:00:00Z', true, now).allowed).toBe(true);
    expect(legacyUsernameChangeAvailability(undefined, false, now).allowed).toBe(true);
  });

  it('keeps legacy disconnect fallback separate from new unknown reasons', () => {
    expect(legacyDisconnectAvailability(undefined, false, 1).allowed).toBe(false);
    expect(legacyDisconnectAvailability(undefined, true, 1).allowed).toBe(true);
    expect(legacyDisconnectAvailability(99 as DisconnectEligibility, false, 1).allowed).toBe(true);
    expect(
      legacyDisconnectAvailability(DisconnectEligibility.DISABLED_BY_OPERATOR, true, 2).allowed
    ).toBe(false);
  });
});
