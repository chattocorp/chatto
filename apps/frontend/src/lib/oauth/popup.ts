export const OAUTH_POPUP_RESPONSE_TYPE = 'chatto:oauth-popup-response';

const OAUTH_POPUP_CHANNEL_PREFIX = 'chatto:oauth-popup:';

export type OAuthPopupResponse = {
  type: typeof OAUTH_POPUP_RESPONSE_TYPE;
  state: string;
  code?: string;
  error?: string;
  errorDescription?: string;
};

/** Return the same-origin channel used by one popup authorization transaction. */
export function oauthPopupChannelName(state: string): string {
  return OAUTH_POPUP_CHANNEL_PREFIX + state;
}

/** Narrow an untrusted cross-window message to the popup response envelope. */
export function isOAuthPopupResponse(value: unknown): value is OAuthPopupResponse {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<OAuthPopupResponse>;
  return candidate.type === OAUTH_POPUP_RESPONSE_TYPE && typeof candidate.state === 'string';
}

/** Build the response envelope from an OAuth callback URL. */
export function oauthPopupResponseFromURL(url: URL): OAuthPopupResponse | null {
  const state = url.searchParams.get('state');
  if (!state) return null;

  const code = url.searchParams.get('code') ?? undefined;
  const error = url.searchParams.get('error') ?? undefined;
  if (!code && !error) return null;

  return {
    type: OAUTH_POPUP_RESPONSE_TYPE,
    state,
    code,
    error,
    errorDescription: url.searchParams.get('error_description') ?? undefined
  };
}
