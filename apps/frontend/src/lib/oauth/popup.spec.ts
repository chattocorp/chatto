import { describe, expect, it } from 'vitest';
import {
  OAUTH_POPUP_RESPONSE_TYPE,
  isOAuthPopupResponse,
  oauthPopupChannelName,
  oauthPopupResponseFromURL
} from './popup';

describe('OAuth popup response', () => {
  it('uses a transaction-specific channel name', () => {
    expect(oauthPopupChannelName('state-123')).toBe('chatto:oauth-popup:state-123');
  });

  it('maps a successful callback URL to a response envelope', () => {
    expect(
      oauthPopupResponseFromURL(
        new URL('https://app.example/servers/callback?mode=popup&code=cht_AC123&state=state-123')
      )
    ).toEqual({
      type: OAUTH_POPUP_RESPONSE_TYPE,
      state: 'state-123',
      code: 'cht_AC123',
      error: undefined,
      errorDescription: undefined
    });
  });

  it('maps an authorization error without requiring a code', () => {
    expect(
      oauthPopupResponseFromURL(
        new URL(
          'https://app.example/servers/callback?mode=popup&error=access_denied&error_description=Nope&state=state-123'
        )
      )
    ).toEqual({
      type: OAUTH_POPUP_RESPONSE_TYPE,
      state: 'state-123',
      code: undefined,
      error: 'access_denied',
      errorDescription: 'Nope'
    });
  });

  it('rejects callbacks and messages without a transaction state', () => {
    expect(
      oauthPopupResponseFromURL(
        new URL('https://app.example/servers/callback?mode=popup&code=cht_AC123')
      )
    ).toBeNull();
    expect(isOAuthPopupResponse({ type: OAUTH_POPUP_RESPONSE_TYPE })).toBe(false);
  });

  it('accepts only the expected response envelope', () => {
    expect(
      isOAuthPopupResponse({ type: OAUTH_POPUP_RESPONSE_TYPE, state: 'state-123', code: 'code' })
    ).toBe(true);
    expect(isOAuthPopupResponse({ type: 'something-else', state: 'state-123' })).toBe(false);
    expect(isOAuthPopupResponse(null)).toBe(false);
  });
});
