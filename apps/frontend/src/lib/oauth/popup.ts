export const OAUTH_POPUP_RESPONSE_TYPE = 'chatto:oauth-popup-response';

const OAUTH_POPUP_CHANNEL_PREFIX = 'chatto:oauth-popup:';
const POPUP_WIDTH = 520;
const POPUP_HEIGHT = 600;
const POPUP_POLL_INTERVAL_MS = 250;
const POPUP_TIMEOUT_MS = 5 * 60 * 1000;

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

export class OAuthPopupError extends Error {}

export type OAuthPopup = {
  response: Promise<OAuthPopupResponse>;
  navigate(url: string): void;
  close(): void;
};

/** Open a same-origin OAuth callback popup while a user gesture is active. */
export function openOAuthPopup(state: string): OAuthPopup {
  const popup = window.open(
    'about:blank',
    `chatto-oauth-${state.slice(0, 12)}`,
    popupFeatures(window)
  );
  if (!popup) throw new OAuthPopupError('The sign-in window could not be opened.');

  const channel =
    typeof BroadcastChannel === 'undefined'
      ? null
      : new BroadcastChannel(oauthPopupChannelName(state));
  if (channel) popup.opener = null;

  return {
    response: waitForPopupResponse(popup, state, channel),
    navigate: (url) => {
      popup.location.href = url;
    },
    close: () => {
      if (!popup.closed) popup.close();
    }
  };
}

function popupFeatures(owner: Window): string {
  const left = Math.max(0, Math.round(owner.screenX + (owner.outerWidth - POPUP_WIDTH) / 2));
  const top = Math.max(0, Math.round(owner.screenY + (owner.outerHeight - POPUP_HEIGHT) / 2));
  return `popup,width=${POPUP_WIDTH},height=${POPUP_HEIGHT},left=${left},top=${top}`;
}

function waitForPopupResponse(
  popup: Window,
  state: string,
  channel: BroadcastChannel | null
): Promise<OAuthPopupResponse> {
  return new Promise((resolve, reject) => {
    let settled = false;

    const cleanup = () => {
      window.removeEventListener('message', handleWindowMessage);
      channel?.close();
      window.clearInterval(closePoll);
      window.clearTimeout(timeout);
    };

    const settle = (response: OAuthPopupResponse) => {
      if (settled || response.state !== state) return;
      settled = true;
      cleanup();
      if (!popup.closed) popup.close();
      resolve(response);
    };

    const fail = (message: string) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new OAuthPopupError(message));
    };

    const handleWindowMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== popup) return;
      if (isOAuthPopupResponse(event.data)) settle(event.data);
    };

    window.addEventListener('message', handleWindowMessage);
    if (channel) {
      channel.onmessage = (event) => {
        if (isOAuthPopupResponse(event.data)) settle(event.data);
      };
    }

    const closePoll = window.setInterval(() => {
      if (popup.closed) fail('The sign-in window was closed before authorization completed.');
    }, POPUP_POLL_INTERVAL_MS);
    const timeout = window.setTimeout(
      () => fail('The sign-in attempt timed out.'),
      POPUP_TIMEOUT_MS
    );
  });
}
