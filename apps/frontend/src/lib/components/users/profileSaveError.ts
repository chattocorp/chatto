import { Code, ConnectError } from '@connectrpc/connect';
import { m } from '$lib/i18n/messages';

/**
 * Map a failed profile save to a user-facing message. A concurrent profile
 * change returns ABORTED from the server; the form keeps its draft and asks
 * the user to reload before saving again.
 */
export function profileSaveErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ConnectError && error.code === Code.Aborted) {
    return m('settings.profile.conflict');
  }
  return error instanceof Error ? error.message : fallback;
}
