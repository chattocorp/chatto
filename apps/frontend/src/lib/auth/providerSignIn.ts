import { resolve } from '$app/paths';
import { generateState } from '$lib/oauth/pkce';
import { openOAuthPopup, type OAuthPopup } from '$lib/oauth/popup';
import { m } from '$lib/i18n/messages';

/**
 * Open provider sign-in during the user's click. The callback reports only
 * completion; the opening client must verify the server's cookie session.
 */
export function openProviderSignIn(loginUrl: string): OAuthPopup {
  const state = generateState();
  const callback = new URL(resolve('/servers/callback'), window.location.origin);
  callback.searchParams.set('mode', 'provider');
  callback.searchParams.set('state', state);
  const providerURL = new URL(loginUrl, window.location.origin);
  providerURL.searchParams.set('redirect', callback.pathname + callback.search);
  const popup = openOAuthPopup(state);
  popup.navigate(providerURL.href);
  return popup;
}

/** Verify provider completion against the origin server before updating the app. */
export async function verifyProviderSignIn(popup: OAuthPopup): Promise<void> {
  const result = await popup.response;
  if (!result.completed || result.error) throw new Error(m('auth.login.failed'));
  const { getCurrentUserViaConnect } = await import('$lib/api-client/viewer');
  await getCurrentUserViaConnect({ baseUrl: '/api/connect', bearerToken: null });
}
