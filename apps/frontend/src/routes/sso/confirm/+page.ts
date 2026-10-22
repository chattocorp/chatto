import { Code, ConnectError } from '@connectrpc/connect';
import { redirect } from '@sveltejs/kit';
import {
  createExternalIdentityFlowAPI,
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
  loadError: 'invalid' | 'failed' | null;
};

/** Load the pending external-identity flow before the confirmation page renders. */
export const load: PageLoad = async ({ url }): Promise<SSOConfirmLoadData> => {
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
        error instanceof ConnectError && error.code === Code.NotFound ? 'invalid' : 'failed'
    };
  }
  if (pending?.invitationRequired)
    redirect(302, `/register?external_identity=${encodeURIComponent(token)}`);
  return pending
    ? { token, pending, loadError: null }
    : { token, pending: null, loadError: 'invalid' };
};
