import { describe, expect, expectTypeOf, it } from 'vitest';
import { Timestamp } from '@bufbuild/protobuf';
import { UsernameChangeAvailability as APIUsernameChangeAvailability } from '@chatto/api-types/api/v1/account_actions_pb';
import {
  UsernameChangeState,
  PasswordChangeAvailability,
  AccountDeletionAvailability,
  UsernameChoiceAvailability,
  DisconnectEligibility,
  type UsernameChangeAvailability,
  usernameChangeAvailabilityFromAPI,
  legacyUsernameChangeAvailability,
  disconnectEligibilityWithFallback
} from '../actionAvailability.js';

describe('action availability', () => {
  it('keeps action enums distinct and avoids contradictory allowed/reason fields', () => {
    expectTypeOf<PasswordChangeAvailability>().not.toExtend<AccountDeletionAvailability>();
    expectTypeOf<DisconnectEligibility>().not.toExtend<PasswordChangeAvailability>();
    expectTypeOf<UsernameChoiceAvailability>().not.toExtend<UsernameChangeState>();
    expectTypeOf<UsernameChangeAvailability>().not.toHaveProperty('allowed');
    expectTypeOf<UsernameChangeAvailability>().not.toHaveProperty('reason');
  });

  it('keeps a missing username decision distinct from an explicit unspecified decision', () => {
    expect(usernameChangeAvailabilityFromAPI()).toBeUndefined();
    expect(usernameChangeAvailabilityFromAPI(new APIUsernameChangeAvailability())).toEqual({
      state: UsernameChangeState.UNSPECIFIED,
      availableAt: undefined
    });
  });

  it('preserves unknown states without granting permission', () => {
    const decision = usernameChangeAvailabilityFromAPI(
      new APIUsernameChangeAvailability({ state: 99 as UsernameChangeState })
    );
    expect(decision?.state).toBe(99);
    expect(decision?.state).not.toBe(UsernameChangeState.ALLOWED);
  });

  it('requires refresh when a cooldown timestamp has expired', () => {
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({
          state: UsernameChangeState.COOLDOWN,
          availableAt: Timestamp.fromDate(new Date('2020-01-01T00:00:00Z'))
        })
      )
    ).toEqual({ state: UsernameChangeState.COOLDOWN, availableAt: '2020-01-01T00:00:00.000Z' });
  });

  it.each([
    UsernameChangeState.ALLOWED,
    UsernameChangeState.PROVIDER_MANAGED,
    UsernameChangeState.UNSPECIFIED,
    99 as UsernameChangeState
  ])('strips cooldown metadata from state %s', (state) => {
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({ state, availableAt: Timestamp.now() })
      )
    ).toEqual({ state, availableAt: undefined });
  });

  it('retains older-server username cooldown and permission bypass behavior', () => {
    const now = Date.parse('2026-01-02T00:00:00Z');
    expect(legacyUsernameChangeAvailability('2026-01-01T00:00:00Z', false, now).state).toBe(
      UsernameChangeState.COOLDOWN
    );
    expect(legacyUsernameChangeAvailability('2026-01-01T00:00:00Z', true, now).state).toBe(
      UsernameChangeState.ALLOWED
    );
    expect(legacyUsernameChangeAvailability(undefined, false, now).state).toBe(
      UsernameChangeState.ALLOWED
    );
  });

  it('uses account state only when the older server did not evaluate disconnect eligibility', () => {
    expect(disconnectEligibilityWithFallback(undefined, false, 1)).toBe(
      DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD
    );
    expect(disconnectEligibilityWithFallback(DisconnectEligibility.UNSPECIFIED, true, 1)).toBe(
      DisconnectEligibility.ALLOWED
    );
    expect(disconnectEligibilityWithFallback(undefined, false, 2)).toBe(
      DisconnectEligibility.ALLOWED
    );
    for (const state of [
      DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD,
      DisconnectEligibility.DISABLED_BY_OPERATOR,
      99 as DisconnectEligibility
    ]) {
      expect(disconnectEligibilityWithFallback(state, true, 2)).toBe(state);
    }
  });
});
