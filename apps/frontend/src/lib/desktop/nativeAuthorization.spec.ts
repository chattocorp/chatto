import { describe, expect, it } from 'vitest';
import { MOBILE_CALLBACK, validateNativeCallback } from './nativeAuthorization';

describe('native authorization callback boundary', () => {
  it('accepts a code only for the current attempt and exact callback', () => {
    expect(
      validateNativeCallback(`${MOBILE_CALLBACK}?state=current&code=code`, 'current').code
    ).toBe('code');
  });

  it('preserves an OAuth error for the current attempt', () => {
    expect(
      validateNativeCallback(`${MOBILE_CALLBACK}?state=current&error=access_denied`, 'current')
        .error
    ).toBe('access_denied');
  });

  it.each([
    'eu.chattocorp.chatto.mobile:/other?state=current&code=code',
    'eu.chattocorp.chatto.mobile://attacker/oauth/callback?state=current&code=code',
    'https://attacker/oauth/callback?state=current&code=code',
    `${MOBILE_CALLBACK}?state=old&code=code`,
    `${MOBILE_CALLBACK}?state=current&state=other&code=code`,
    `${MOBILE_CALLBACK}?state=current&code=one&code=two`,
    `${MOBILE_CALLBACK}?state=current&code=code&error=access_denied`,
    `${MOBILE_CALLBACK}?state=current`,
    `${MOBILE_CALLBACK}?state=current&code=code#fragment`
  ])('rejects an unrelated or ambiguous callback: %s', (callback) => {
    expect(() => validateNativeCallback(callback, 'current')).toThrow();
  });
});
