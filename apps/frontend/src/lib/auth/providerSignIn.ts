import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { saveReturnUrl } from './returnNavigation';
import { generateState } from '$lib/oauth/pkce';
import { openOAuthPopup, type OAuthPopup } from '$lib/oauth/popup';
import { m } from '$lib/i18n/messages';

/**
 * The provider's sign-in URL. `loginHint` is the identity to use, such as an
 * AT Protocol handle; providers without hints ignore it (FDR-056).
 */
export function providerLoginUrl(loginUrl: string, loginHint?: string): string {
  if (!loginHint) return loginUrl;
  const url = new URL(loginUrl, window.location.origin);
  url.searchParams.set('login_hint', loginHint);
  return url.origin === window.location.origin ? url.pathname + url.search : url.href;
}

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

/** A provider sign-in that the server rejected with a login error code. */
export class ProviderSignInError extends Error {
  constructor(readonly code: string) {
    super(m('auth.login.failed'));
  }
}

/**
 * Outcome of a provider popup. `confirm-account` means that the provider
 * identity has no account yet: the opening window confirms a new one.
 */
export type ProviderSignInResult =
  { kind: 'signed-in' } | { kind: 'confirm-account'; token: string };

/** Verify provider completion against the origin server before updating the app. */
export async function verifyProviderSignIn(
  popup: Pick<OAuthPopup, 'response'>
): Promise<ProviderSignInResult> {
  const result = await popup.response;
  if (result.error) throw new ProviderSignInError(result.error);
  if (result.externalIdentityToken) {
    return { kind: 'confirm-account', token: result.externalIdentityToken };
  }
  if (!result.completed) throw new Error(m('auth.login.failed'));
  const { getCurrentUserViaConnect } = await import('@chatto/client/api/viewer');
  await getCurrentUserViaConnect({ baseUrl: '/api/connect', bearerToken: null });
  return { kind: 'signed-in' };
}

/**
 * Show the new-account confirmation in this window. After the account exists,
 * origin authentication continues to `returnPath`.
 */
export async function confirmProviderAccount(token: string, returnPath: string): Promise<void> {
  // The default destinations need no queued return. A queued `/` would make
  // the chat landing route wait for a return navigation that never arrives.
  if (returnPath !== '/' && returnPath !== '/chat') saveReturnUrl(returnPath);
  await goto(resolve(`/sso/confirm?token=${encodeURIComponent(token)}`));
}
