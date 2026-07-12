import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { isAuthenticationRequiredError } from './errors';

describe('isAuthenticationRequiredError', () => {
  it('detects Connect unauthenticated errors', () => {
    expect(
      isAuthenticationRequiredError(new ConnectError('session expired', Code.Unauthenticated))
    ).toBe(true);
  });

  it('does not classify typed unavailable errors by their message', () => {
    expect(
      isAuthenticationRequiredError(
        new ConnectError('authentication required: storage unavailable', Code.Unavailable)
      )
    ).toBe(false);
  });

  it('keeps the combined message fallback for older clients and transports', () => {
    expect(isAuthenticationRequiredError({ message: 'authentication required' })).toBe(true);
  });

  it('ignores unrelated errors', () => {
    expect(isAuthenticationRequiredError({ message: 'network unavailable' })).toBe(false);
  });
});
