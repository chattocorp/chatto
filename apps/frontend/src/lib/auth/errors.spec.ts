import { describe, expect, it } from 'vitest';
import { isAuthenticationRequiredError } from './errors';

describe('isAuthenticationRequiredError', () => {
  it('detects structured unauthenticated GraphQL errors', () => {
    expect(
      isAuthenticationRequiredError({
        graphQLErrors: [{ extensions: { code: 'UNAUTHENTICATED' } }]
      })
    ).toBe(true);
  });

  it('keeps the legacy GraphQL message fallback for older servers', () => {
    expect(
      isAuthenticationRequiredError({
        graphQLErrors: [{ message: 'authentication required' }]
      })
    ).toBe(true);
  });

  it('keeps the combined message fallback for older clients and transports', () => {
    expect(isAuthenticationRequiredError({ message: '[GraphQL] authentication required' })).toBe(
      true
    );
  });

  it('ignores unrelated errors', () => {
    expect(
      isAuthenticationRequiredError({
        graphQLErrors: [{ message: 'network unavailable', extensions: { code: 'NETWORK_ERROR' } }]
      })
    ).toBe(false);
  });
});
