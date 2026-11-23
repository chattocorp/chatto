/**
 * Fresh-auth re-verification: a new sign-in through an OIDC provider that is
 * already linked to the account (FDR-023). On servers without password proof,
 * account settings and the identity-link window use it when the server
 * requires a recent sign-in, then continue the action that needed it.
 */
import { m } from '$lib/i18n/messages';
import type { ExternalIdentityProviderInfo } from '$lib/api/externalIdentities';

/** Query parameter that the provider callback adds after a successful re-verification. */
export const REVERIFIED_PARAM = 'reverified';

/** Whether the provider callback reported a successful re-verification. */
export function wasReverified(url: URL): boolean {
  return url.searchParams.get(REVERIFIED_PARAM) === '1';
}

/**
 * Browser URL that re-verifies the user through provider and then returns to
 * redirectPath. The URL is always on this page's origin: the server accepts
 * only a same-origin navigation from the browser that holds the session.
 */
export function reverificationStartUrl(
  provider: ExternalIdentityProviderInfo,
  redirectPath: string
): string {
  const url = new URL(
    `/auth/providers/${encodeURIComponent(provider.id)}/reverify`,
    globalThis.location.origin
  );
  url.searchParams.set('redirect', redirectPath);
  return url.href;
}

/** Linked providers that can confirm a recent sign-in for the caller. */
export function reverificationProviders(
  providers: readonly ExternalIdentityProviderInfo[]
): ExternalIdentityProviderInfo[] {
  return providers.filter((provider) => provider.linked && provider.reverificationAvailable);
}

/** Message for an error code from a re-verification callback. */
export function reverificationErrorMessage(errorCode: string): string {
  switch (errorCode) {
    case 'reverification_mismatch':
      return m('settings.account.sso.reverification_mismatch');
    case 'provider_access_denied':
      return m('auth.login.error.provider_access_denied');
    default:
      return m('settings.account.sso.reverification_failed');
  }
}
