import { Code, ConnectError } from '@connectrpc/connect';
import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import {
  createExternalIdentityFlowAPI,
  ExternalIdentityFlowKind,
  type PendingExternalIdentityInfo
} from '$lib/api/externalIdentities';
import type { PageLoad } from './$types';

/** Settled result of loading an external-identity confirmation flow. */
export type SSOConfirmLoadData = {
  /** Confirmation token from the URL, if present. */
  token: string;
  /** Pending confirmation flow, when the token is valid. */
  pending: PendingExternalIdentityInfo | null;
  /** Stable error code for the confirmation page, if loading did not succeed. */
  loadError: 'invalid' | 'failed' | 'provider_username' | null;
  /** The user chose to link this provider through an existing account. */
  existingAccount?: boolean;
};

/** Load the pending external-identity flow before the confirmation page renders. */
export const load: PageLoad = async ({ url, parent }): Promise<SSOConfirmLoadData> => {
  const token = url.searchParams.get('token') ?? '';
  if (!token) {
    return { token, pending: null, loadError: 'invalid' };
  }

  let pending: PendingExternalIdentityInfo | null;
  try {
    pending = await createExternalIdentityFlowAPI().getPending(token);
  } catch (error) {
    return {
      token,
      pending: null,
      loadError:
        error instanceof ConnectError && error.code === Code.NotFound
          ? 'invalid'
          : error instanceof ConnectError && error.code === Code.FailedPrecondition
            ? 'provider_username'
            : 'failed'
    };
  }
  if (!pending) return { token, pending: null, loadError: 'invalid' };

  if (
    url.searchParams.get('existing') === '1' &&
    pending.kind === ExternalIdentityFlowKind.CREATE_ACCOUNT
  ) {
    const { user } = await parent();
    if (!user) {
      redirect(
        302,
        `${resolve('/login')}?redirect=${encodeURIComponent(url.pathname + url.search)}`
      );
    }
    return { token, pending, loadError: null, existingAccount: true };
  }
  return { token, pending, loadError: null };
};
