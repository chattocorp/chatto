/**
 * The identity-link window contract: how account settings, the link window
 * page (`/sso/link`), and the provider callback hand a provider link to each
 * other (FDR-023).
 *
 * Account settings open the link window. The window starts the link, asks for
 * the current password when the server requires fresh proof, and receives the
 * provider callback. After a successful link, it announces the link to the
 * account settings pages on its origin.
 */
import { Code, ConnectError } from '@connectrpc/connect';
import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';
import { errorMessage } from '$lib/utils/errorMessage';
import { wasReverified } from '$lib/auth/freshAuthReverification';

/**
 * Account settings path that older servers accept for a link window. Newer
 * servers forward it, with its query, to the link window page.
 */
const LEGACY_IDENTITY_LINK_PATH = '/chat/-/settings/account';

/** BroadcastChannel on which the link window announces a linked account ID. */
const PROVIDER_LINKED_CHANNEL = 'chatto-provider-linked';

const PROVIDER_PARAM = 'link_provider';
const USER_PARAM = 'link_user';
const COMPLETE_PARAM = 'link_complete';
const ERROR_PARAM = 'error';

/** The link that a link window URL describes. */
export type IdentityLinkRequest = {
  providerId: string;
  /** Account that started the link. The window refuses another account. */
  userId: string;
  /** The provider callback returned to the window. */
  completed: boolean;
  /** Link or re-verification error code from the server's callback, or empty. */
  errorCode: string;
  /** A fresh-auth re-verification succeeded just before the window opened. */
  reverified: boolean;
};

/** Whether a URL carries link window parameters. */
export function hasIdentityLinkParams(url: URL): boolean {
  return url.searchParams.has(PROVIDER_PARAM) || url.searchParams.has(USER_PARAM);
}

/** Read the link that a link window URL describes. */
export function parseIdentityLinkRequest(url: URL): IdentityLinkRequest {
  return {
    providerId: url.searchParams.get(PROVIDER_PARAM) ?? '',
    userId: url.searchParams.get(USER_PARAM) ?? '',
    completed: url.searchParams.get(COMPLETE_PARAM) === '1',
    errorCode: url.searchParams.get(ERROR_PARAM) ?? '',
    reverified: wasReverified(url)
  };
}

function withLinkParams(path: string, base: string, providerId: string, userId: string): URL {
  const url = new URL(path, base);
  url.searchParams.set(PROVIDER_PARAM, providerId);
  url.searchParams.set(USER_PARAM, userId);
  return url;
}

/**
 * URL of the link window on another server. It uses the legacy settings path,
 * because older servers have no link window page.
 */
export function remoteIdentityLinkUrl(serverBaseUrl: string, providerId: string, userId: string) {
  return withLinkParams(LEGACY_IDENTITY_LINK_PATH, serverBaseUrl, providerId, userId).href;
}

/** Whether an internal path opens the link window. */
export function isIdentityLinkPath(path: string): boolean {
  return new URL(path, 'http://localhost').pathname === resolve('/sso/link');
}

/** Internal path that starts the link again in the link window. */
export function identityLinkPath(providerId: string, userId: string): string {
  const url = withLinkParams(resolve('/sso/link'), 'http://localhost', providerId, userId);
  return url.pathname + url.search;
}

/**
 * Internal path to which the provider callback returns. It uses the legacy
 * settings path: during a rolling upgrade, an older replica can serve the
 * callback, and it has no link window page. Newer frontends forward the path
 * to the link window.
 */
export function identityLinkReturnPath(providerId: string, userId: string): string {
  const url = withLinkParams(LEGACY_IDENTITY_LINK_PATH, 'http://localhost', providerId, userId);
  url.searchParams.set(COMPLETE_PARAM, '1');
  return url.pathname + url.search;
}

/** Message for a link error code from the provider callback. */
export function identityLinkErrorMessage(errorCode: string): string {
  switch (errorCode) {
    case 'provider_access_denied':
      return m('auth.login.error.provider_access_denied');
    case 'external_identity_conflict':
      return m('auth.login.error.external_identity_conflict');
    default:
      return m('settings.account.sso.link_failed');
  }
}

/** What to show after a link start failed. */
export type LinkStartFailure =
  /** The server requires fresh proof, and the user can give the current password. */
  | { kind: 'password_required' }
  /** The server refused the current password that the user gave. */
  | { kind: 'password_rejected'; message: string }
  /** The server requires fresh proof, and a linked provider can re-verify the user. */
  | { kind: 'reverification_required' }
  /**
   * Any other failure. `retryable` is false when the same request cannot
   * succeed, for example when only a new provider sign-in gives fresh proof.
   */
  | { kind: 'failed'; message: string; retryable: boolean };

/** Decide how to show a failed link start. */
export function linkStartFailure(
  error: unknown,
  options: {
    /** The request included the current password. */
    passwordSent: boolean;
    /** The account has a password and the server allows password sign-in. */
    canUsePasswordProof: boolean;
    directLoginEnabled: boolean;
    /**
     * A linked provider can re-verify the user, and no re-verification
     * succeeded just before. Without it, the failure explains how to sign in again.
     */
    canReverify?: boolean;
  }
): LinkStartFailure {
  const fallback = m('settings.account.sso.link_failed');
  if (options.passwordSent) {
    return { kind: 'password_rejected', message: errorMessage(error, fallback) };
  }
  if (error instanceof ConnectError && error.code === Code.FailedPrecondition) {
    if (options.canUsePasswordProof) return { kind: 'password_required' };
    if (options.canReverify) return { kind: 'reverification_required' };
    return {
      kind: 'failed',
      retryable: false,
      message: options.directLoginEnabled
        ? m('settings.account.sso.fresh_auth_required')
        : m('settings.account.sso.sso_fresh_auth_required')
    };
  }
  return { kind: 'failed', message: errorMessage(error, fallback), retryable: true };
}

/** Tell the account settings pages on this origin that an account linked a provider. */
export function announceProviderLinked(userId: string): void {
  if (typeof BroadcastChannel === 'undefined') return;
  const channel = new BroadcastChannel(PROVIDER_LINKED_CHANNEL);
  channel.postMessage(userId);
  channel.close();
}

/**
 * Call `onLinked` with each account ID that a link window on this origin
 * announces. Returns the cleanup function.
 */
export function onProviderLinked(onLinked: (userId: string) => void): () => void {
  if (typeof BroadcastChannel === 'undefined') return () => {};
  const channel = new BroadcastChannel(PROVIDER_LINKED_CHANNEL);
  channel.onmessage = ({ data }) => {
    if (typeof data === 'string') onLinked(data);
  };
  return () => channel.close();
}
