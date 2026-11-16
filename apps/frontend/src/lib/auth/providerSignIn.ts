import { resolve } from '$app/paths';
import { randomOAuthValue } from '@chatto/client/auth/oauth';
import { openOAuthPopup, type OAuthPopup } from '$lib/oauth/popup';
import { m } from '$lib/i18n/messages';

/**
 * Open provider sign-in during the user's click. The callback reports only
 * completion; the opening client must verify the server's cookie session.
 */
export function openProviderSignIn(loginUrl: string): OAuthPopup {
  const state = randomOAuthValue(32);
  const callback = new URL(resolve('/servers/callback'), window.location.origin);
  callback.searchParams.set('mode', 'provider');
  callback.searchParams.set('state', state);
  const providerURL = new URL(loginUrl, window.location.origin);
  providerURL.searchParams.set('redirect', callback.pathname + callback.search);
  const popup = openOAuthPopup(state);
  void popup.navigate(providerURL.href);
  return popup;
}

/** Verify provider completion against the origin server before updating the app. */
export async function verifyProviderSignIn(popup: OAuthPopup): Promise<void> {
  const result = await popup.response;
  if (!result.completed || result.error) throw new Error(m('auth.login.failed'));
  const { getCurrentUserViaConnect } = await import('@chatto/client/api/viewer');
  await getCurrentUserViaConnect({ baseUrl: '/api/connect', bearerToken: null });
}
