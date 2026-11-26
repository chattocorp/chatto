import { isOAuthCallback } from '@chatto/client/auth/oauth';
import {
  oauthPopupResponseFromURL,
  type OAuthPopup,
  type OAuthPopupResponse
} from '$lib/oauth/popup';
import { OAuthPopupError } from '$lib/oauth/popupError';

export const DESKTOP_CALLBACK = 'chatto://desktop/servers/callback?mode=popup';

/** Use the installed host to deliver a callback to the same pending PKCE exchange as web sign-in. */
export function desktopBrowserAuthorization(origin: string, state: string): OAuthPopup | undefined {
  const bridge = window.chattoDesktop?.browserAuthorization;
  if (!bridge) return;
  const result = Promise.withResolvers<OAuthPopupResponse>();
  let started = false;
  let cancelled = false;
  function cancel() {
    if (cancelled) return;
    cancelled = true;
    if (started) bridge?.cancel(state);
    result.reject(new OAuthPopupError('The sign-in attempt was cancelled.', 'cancelled'));
  }
  return {
    response: result.promise,
    cancel,
    close: cancel,
    navigate(url) {
      if (!cancelled) {
        started = true;
        bridge
          .start(origin, url)
          .then((callback) => {
            if (typeof callback !== 'string')
              throw new OAuthPopupError(
                'Cannot register the Desktop sign-in callback.',
                callback.error
              );
            if (!isOAuthCallback({ origin, state, redirectUri: DESKTOP_CALLBACK }, callback))
              throw new OAuthPopupError('Invalid browser sign-in callback.');
            const response = oauthPopupResponseFromURL(new URL(callback));
            if (!response) throw new OAuthPopupError('Invalid browser sign-in callback.');
            result.resolve(response);
          })
          .catch(result.reject);
      }
      // The caller waits on response, so Cancel can finish while the host is still closing.
      return Promise.resolve();
    }
  };
}
