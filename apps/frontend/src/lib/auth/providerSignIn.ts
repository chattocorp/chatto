import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { navigateAfterAuthentication, saveReturnUrl } from './returnNavigation';
import { isProviderContinuation } from './providerContinuation';
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

/**
 * Sign in to this server with an AT Protocol identity and continue to
 * `redirectPath` like a password sign-in. Call this from the user's action:
 * a sign-in to the PDS opens a popup window. A new identity continues with the
 * account confirmation in this window.
 */
export async function signInWithAtprotoOnOrigin(
  identifier: string,
  redirectPath: string,
  admission?: string
): Promise<void> {
  const { signInToOriginWithAtproto } = await import('$lib/atproto/signIn');
  const result = await signInToOriginWithAtproto(identifier, { admission });
  if (result.kind === 'confirm-account') {
    await confirmProviderAccount(result.token, redirectPath);
    return;
  }
  const { completeOriginAuthentication } = await import('./originAuthentication');
  const resumed = await completeOriginAuthentication({
    resumeReturnNavigation: !isProviderContinuation(redirectPath)
  });
  if (!resumed) await navigateAfterAuthentication(redirectPath);
}
