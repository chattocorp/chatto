import { OAuthPopupError } from '$lib/oauth/popupError';
import { openAuthorizationWindow, type AuthorizationWindow } from './authorizationWindow';

export const OAUTH_POPUP_RESPONSE_TYPE = 'chatto:oauth-popup-response';

const OAUTH_POPUP_CHANNEL_PREFIX = 'chatto:oauth-popup:';
const POPUP_WIDTH = 520;
const POPUP_HEIGHT = 600;
const POPUP_POLL_INTERVAL_MS = 250;
const POPUP_TIMEOUT_MS = 5 * 60 * 1000;

export type OAuthPopupResponse = {
  type: typeof OAUTH_POPUP_RESPONSE_TYPE;
  state: string;
  /** Provider sign-in established an origin cookie session; verify it with the server. */
  completed?: boolean;
  code?: string;
  error?: string;
  errorDescription?: string;
  /** The `iss` parameter of the callback: the server that sent the response. */
  iss?: string;
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
  // More than one issuer makes the source of the response ambiguous.
  if (!state || url.searchParams.getAll('iss').length > 1) return null;

  if (url.searchParams.get('mode') === 'provider' && !url.searchParams.has('error')) {
    return { type: OAUTH_POPUP_RESPONSE_TYPE, state, completed: true };
  }

  const code = url.searchParams.get('code') ?? undefined;
  const error = url.searchParams.get('error') ?? undefined;
  if (!code && !error) return null;

  return {
    type: OAUTH_POPUP_RESPONSE_TYPE,
    state,
    code,
    error,
    errorDescription: url.searchParams.get('error_description') ?? undefined,
    iss: url.searchParams.get('iss') ?? undefined
  };
}

export { OAuthPopupError } from './popupError';

export type OAuthPopup = {
  /**
   * The callback response of this transaction. Rejects with `OAuthPopupError`
   * when the person closes the window (`cancelled`) or the attempt times out.
   */
  response: Promise<OAuthPopupResponse>;
  /** Send the window to the authorization URL. */
  navigate(url: string): Promise<void>;
  /** Close the window. A pending `response` then rejects as `cancelled`. */
  close(): void;
};

/**
 * Open an OAuth popup while a user gesture is active, and wait for the
 * callback response of the transaction `state`. Call it before any `await`,
 * or the browser can block the window. `features` sets the window size.
 *
 * @throws OAuthPopupError with reason `blocked` when the browser blocks the window.
 */
export function openOAuthPopup(state: string, features = popupFeatures(window)): OAuthPopup {
  const authorizationWindow = openAuthorizationWindow(
    `chatto-oauth-${state.slice(0, 12)}`,
    features
  );
  if (!authorizationWindow)
    throw new OAuthPopupError('The sign-in window could not be opened.', 'blocked');

  const channel =
    typeof BroadcastChannel === 'undefined'
      ? null
      : new BroadcastChannel(oauthPopupChannelName(state));
  // The callback returns through BroadcastChannel, so the untrusted remote
  // page does not need a reference capable of navigating the main client.
  if (channel) authorizationWindow.detachOpener();

  return {
    response: waitForPopupResponse(authorizationWindow, state, channel),
    navigate: (url) => authorizationWindow.navigate(url),
    close: () => {
      void closeAuthorizationWindow(authorizationWindow);
    }
  };
}

function popupFeatures(owner: Window): string {
  const left = Math.max(0, Math.round(owner.screenX + (owner.outerWidth - POPUP_WIDTH) / 2));
  const top = Math.max(0, Math.round(owner.screenY + (owner.outerHeight - POPUP_HEIGHT) / 2));
  return `popup,width=${POPUP_WIDTH},height=${POPUP_HEIGHT},left=${left},top=${top}`;
}

function waitForPopupResponse(
  authorizationWindow: AuthorizationWindow,
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
      void closeAuthorizationWindow(authorizationWindow);
      resolve(response);
    };

    const fail = (message: string, reason: 'failed' | 'cancelled' = 'failed') => {
      if (settled) return;
      settled = true;
      cleanup();
      void closeAuthorizationWindow(authorizationWindow);
      reject(new OAuthPopupError(message, reason));
    };

    const handleWindowMessage = (event: MessageEvent) => {
      if (
        event.origin !== window.location.origin ||
        !authorizationWindow.messageSource ||
        event.source !== authorizationWindow.messageSource
      )
        return;
      if (isOAuthPopupResponse(event.data)) settle(event.data);
    };

    window.addEventListener('message', handleWindowMessage);
    if (channel) {
      channel.onmessage = (event) => {
        if (isOAuthPopupResponse(event.data)) settle(event.data);
      };
    }

    let polling = false;
    const closePoll = window.setInterval(async () => {
      if (polling || settled) return;
      polling = true;
      try {
        if (await authorizationWindow.isClosed()) {
          fail('The sign-in window was closed before authorization completed.', 'cancelled');
        }
      } catch {
        fail('The sign-in window could not be inspected.');
      } finally {
        polling = false;
      }
    }, POPUP_POLL_INTERVAL_MS);
    const timeout = window.setTimeout(
      () => fail('The sign-in attempt timed out.'),
      POPUP_TIMEOUT_MS
    );
  });
}

async function closeAuthorizationWindow(authorizationWindow: AuthorizationWindow): Promise<void> {
  try {
    await authorizationWindow.close();
  } catch {
    // Closing is best-effort after the flow has already completed or failed.
  }
}
