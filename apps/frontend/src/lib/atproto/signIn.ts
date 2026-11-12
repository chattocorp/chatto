/**
 * Account link records of the device's AT Protocol session (FDR-056).
 *
 * Chatto servers sign users in with their own AT Protocol OAuth flow and
 * publish the DID of the linked identity. Only the user's own device can
 * write the account link record that lets other clients verify that DID,
 * because servers never get write access to the user's repository (ADR-123).
 */

import {
  deleteAccountLinkRecord,
  writeAccountLinkRecord,
  type ChattoAccountRef
} from '@chatto/client/atproto/accountLink';
import { atprotoLinkVerifier, atprotoRepoSession } from './session.svelte';

/**
 * Whether this app can write link records for `account`. The device session
 * of each server's app may write only that server's collection (ADR-123).
 */
export function canWriteAtprotoLinkRecord(account: ChattoAccountRef): boolean {
  return new URL(account.serverOrigin).origin === window.location.origin;
}

/**
 * Write the account link record for `account` when the device session
 * belongs to `did` and other clients cannot verify the link yet. Without a
 * matching device session, or for another server's account, this does
 * nothing.
 */
export async function ensureAtprotoLinkRecord(
  account: ChattoAccountRef,
  did: string
): Promise<void> {
  if (!canWriteAtprotoLinkRecord(account)) return;
  const session = await atprotoRepoSession(did);
  if (!session) return;
  atprotoLinkVerifier.forget(did, account);
  const verification = await atprotoLinkVerifier.verify(did, account);
  if (verification.status === 'verified') return;
  await writeAccountLinkRecord(session, account);
  atprotoLinkVerifier.forget(did, account);
}

/**
 * Delete the account link record when the device session belongs to `did`
 * and `account` is on this app's server.
 */
export async function removeAtprotoLinkRecord(
  account: ChattoAccountRef,
  did: string
): Promise<void> {
  if (!canWriteAtprotoLinkRecord(account)) return;
  const session = await atprotoRepoSession(did);
  if (!session) return;
  await deleteAccountLinkRecord(session, account);
  atprotoLinkVerifier.forget(did, account);
}
