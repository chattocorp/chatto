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
  canChangeUsername,
  disconnectEligibilityWithFallback,
  passwordChangeAvailabilityWithFallback,
  accountDeletionAvailabilityWithFallback
} from '../actionAvailability.js';

describe('action availability', () => {
  it.each([true, false])(
    'uses legacy password and deletion policy only for missing decisions (%s)',
    (allowed) => {
      expect(passwordChangeAvailabilityWithFallback(undefined, allowed)).toBe(
        allowed
          ? PasswordChangeAvailability.ALLOWED
          : PasswordChangeAvailability.DIRECT_LOGIN_DISABLED
      );
      expect(accountDeletionAvailabilityWithFallback(undefined, allowed)).toBe(
        allowed
          ? AccountDeletionAvailability.ALLOWED
          : AccountDeletionAvailability.PERMISSION_DENIED
      );
      for (const state of [
        PasswordChangeAvailability.UNSPECIFIED,
        PasswordChangeAvailability.ALLOWED,
        PasswordChangeAvailability.DIRECT_LOGIN_DISABLED,
        PasswordChangeAvailability.UNSUPPORTED_ACCOUNT,
        99 as PasswordChangeAvailability
      ]) {
        expect(passwordChangeAvailabilityWithFallback(state, allowed)).toBe(state);
      }
      for (const state of [
        AccountDeletionAvailability.UNSPECIFIED,
        AccountDeletionAvailability.ALLOWED,
        AccountDeletionAvailability.PERMISSION_DENIED,
        AccountDeletionAvailability.UNSUPPORTED_ACCOUNT,
        99 as AccountDeletionAvailability
      ]) {
        expect(accountDeletionAvailabilityWithFallback(state, allowed)).toBe(state);
      }
    }
  );

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

  it('permits an allowed action when its time constraint expires', () => {
    const deadline = Date.parse('2026-01-01T00:00:00Z');
    const action = {
      state: UsernameChangeState.ALLOWED,
      availableAt: new Date(deadline).toISOString()
    };
    expect(canChangeUsername(action, deadline - 1)).toBe(false);
    expect(canChangeUsername(action, deadline)).toBe(true);
    expect(canChangeUsername(action, deadline + 1)).toBe(true);
    expect(canChangeUsername({ state: UsernameChangeState.ALLOWED }, deadline)).toBe(true);
    expect(canChangeUsername({ ...action, availableAt: 'invalid' }, deadline)).toBe(false);
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({
          state: UsernameChangeState.ALLOWED,
          availableAt: Timestamp.fromDate(new Date('2020-01-01T00:00:00Z'))
        })
      )
    ).toEqual({ state: UsernameChangeState.ALLOWED, availableAt: '2020-01-01T00:00:00.000Z' });
  });

  it.each([
    UsernameChangeState.ALLOWED,
    UsernameChangeState.PROVIDER_MANAGED,
    UsernameChangeState.UNSPECIFIED,
    99 as UsernameChangeState
  ])('preserves independent timing for state %s without overriding policy', (state) => {
    const timestamp = Timestamp.fromDate(new Date('2026-01-01T00:00:00Z'));
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({ state, availableAt: timestamp })
      )
    ).toEqual({ state, availableAt: timestamp.toDate().toISOString() });
    for (const availableAt of [undefined, '2020-01-01T00:00:00Z', '2030-01-01T00:00:00Z']) {
      const allowed = canChangeUsername({ state, availableAt }, Date.parse('2026-01-01T00:00:00Z'));
      expect(allowed).toBe(
        state === UsernameChangeState.ALLOWED && availableAt !== '2030-01-01T00:00:00Z'
      );
    }
  });

  it('retains older-server username cooldown and permission bypass behavior', () => {
    const now = Date.parse('2026-01-02T00:00:00Z');
    const waiting = legacyUsernameChangeAvailability('2026-01-01T00:00:00Z', false, now);
    expect(waiting.state).toBe(UsernameChangeState.ALLOWED);
    expect(canChangeUsername(waiting, now)).toBe(false);
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
