import { describe, expect, expectTypeOf, it } from 'vitest';
import { Timestamp } from '@bufbuild/protobuf';
import {
  UsernameChangeAvailability as APIUsernameChangeAvailability,
  PasswordChangeAvailability as APIPasswordChangeAvailability,
  AccountDeletionAvailability as APIAccountDeletionAvailability
} from '@chatto/api-types/api/v1/account_actions_pb';
import { IdentityDisconnectAvailability as APIIdentityDisconnectAvailability } from '@chatto/api-types/api/v1/external_identities_pb';
import { UsernameChoiceAvailability as APIUsernameChoiceAvailability } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';
import {
  UsernameChangeReason,
  PasswordChangeReason,
  AccountDeletionReason,
  IdentityDisconnectReason,
  UsernameChoiceReason,
  passwordChangeAvailabilityFromAPI,
  accountDeletionAvailabilityFromAPI,
  identityDisconnectAvailabilityFromAPI,
  usernameChoiceAvailabilityFromAPI,
  type UsernameChangeAvailability,
  type PasswordChangeAvailability,
  type AccountDeletionAvailability,
  type IdentityDisconnectAvailability,
  type UsernameChoiceAvailability,
  usernameChangeAvailabilityFromAPI,
  legacyUsernameChangeAvailability,
  legacyDisconnectAvailability
} from '../actionAvailability.js';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';

describe('action availability', () => {
  it('keeps reason types and timing metadata scoped to their actions', () => {
    expectTypeOf<PasswordChangeAvailability>().not.toExtend<UsernameChangeAvailability>();
    expectTypeOf<IdentityDisconnectAvailability>().not.toExtend<PasswordChangeAvailability>();
    expectTypeOf<AccountDeletionAvailability>().not.toExtend<IdentityDisconnectAvailability>();
    expectTypeOf<UsernameChoiceAvailability>().not.toExtend<UsernameChangeAvailability>();
    expectTypeOf<PasswordChangeAvailability>().not.toHaveProperty('availableAt');
    expectTypeOf<IdentityDisconnectAvailability>().not.toHaveProperty('availableAt');
    expectTypeOf<UsernameChoiceAvailability>().not.toHaveProperty('availableAt');
    expectTypeOf<AccountDeletionAvailability>().not.toHaveProperty('availableAt');
  });

  it('preserves missing and unknown decisions for each non-cooldown action', () => {
    expect(passwordChangeAvailabilityFromAPI()).toBeUndefined();
    expect(accountDeletionAvailabilityFromAPI()).toBeUndefined();
    expect(identityDisconnectAvailabilityFromAPI()).toBeUndefined();
    expect(usernameChoiceAvailabilityFromAPI()).toBeUndefined();
    const decisions = [
      passwordChangeAvailabilityFromAPI(
        new APIPasswordChangeAvailability({ reason: 99 as PasswordChangeReason })
      ),
      accountDeletionAvailabilityFromAPI(
        new APIAccountDeletionAvailability({ reason: 99 as AccountDeletionReason })
      ),
      identityDisconnectAvailabilityFromAPI(
        new APIIdentityDisconnectAvailability({ reason: 99 as IdentityDisconnectReason })
      ),
      usernameChoiceAvailabilityFromAPI(
        new APIUsernameChoiceAvailability({ reason: 99 as UsernameChoiceReason })
      )
    ];
    for (const decision of decisions) expect(decision).toEqual({ allowed: false, reason: 99 });
  });

  it('clears blocking metadata independently for every allowed action', () => {
    const decisions = [
      passwordChangeAvailabilityFromAPI(
        new APIPasswordChangeAvailability({
          allowed: true,
          reason: PasswordChangeReason.DIRECT_LOGIN_DISABLED
        })
      ),
      accountDeletionAvailabilityFromAPI(
        new APIAccountDeletionAvailability({
          allowed: true,
          reason: AccountDeletionReason.PERMISSION_DENIED
        })
      ),
      identityDisconnectAvailabilityFromAPI(
        new APIIdentityDisconnectAvailability({
          allowed: true,
          reason: IdentityDisconnectReason.LAST_SIGN_IN_METHOD
        })
      ),
      usernameChoiceAvailabilityFromAPI(
        new APIUsernameChoiceAvailability({
          allowed: true,
          reason: UsernameChoiceReason.PROVIDER_MANAGED
        })
      )
    ];
    for (const decision of decisions) expect(decision).toEqual({ allowed: true, reason: 0 });
  });

  it('keeps absence distinct from a denied action', () => {
    expect(usernameChangeAvailabilityFromAPI()).toBeUndefined();
    expect(usernameChangeAvailabilityFromAPI(new APIUsernameChangeAvailability())?.allowed).toBe(
      false
    );
  });

  it('preserves unknown blocking reasons without enabling the action', () => {
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({ allowed: false, reason: 99 as UsernameChangeReason })
      )
    ).toMatchObject({ allowed: false, reason: 99 });
  });

  it('does not turn an expired cooldown snapshot into permission', () => {
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({
          allowed: false,
          reason: UsernameChangeReason.COOLDOWN,
          availableAt: Timestamp.fromDate(new Date('2020-01-01T00:00:00Z'))
        })
      )
    ).toMatchObject({ allowed: false, availableAt: '2020-01-01T00:00:00.000Z' });
  });

  it('discards denial metadata from allowed snapshots', () => {
    expect(
      usernameChangeAvailabilityFromAPI(
        new APIUsernameChangeAvailability({
          allowed: true,
          reason: UsernameChangeReason.COOLDOWN,
          availableAt: Timestamp.now()
        })
      )
    ).toEqual({
      allowed: true,
      reason: UsernameChangeReason.UNSPECIFIED,
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
