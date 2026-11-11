/**
 * AT Protocol sign-in and linking on Chatto servers (FDR-056).
 *
 * The device's AT Protocol session asks the user's PDS for a short-lived
 * service token addressed to one Chatto server. The server verifies the token
 * and signs the user in, starts a new-account confirmation, or links the
 * identity. A linked identity is the user's published DID. After a sign-in or
 * link, the device writes the account link record that lets other clients
 * verify the handle.
 */

import type { OAuthSession } from '@atproto/oauth-client-browser';
import {
  atprotoServiceAudience,
  deleteAccountLinkRecord,
  writeAccountLinkRecord,
  type ChattoAccountRef
} from '@chatto/client/atproto/accountLink';
import { browserCookieAuthenticationHeaders } from '@chatto/client/auth/authenticationMode';
import { directBearerSession, type NewBearerSession } from '@chatto/client/auth/bearerSession';
import {
  atprotoLinkVerifier,
  atprotoRepoSession,
  atprotoServiceToken,
  ensureAtprotoSession
} from './session.svelte';

/** A server rejected an AT Protocol sign-in with a stable error code. */
export class AtprotoSignInError extends Error {
  constructor(readonly code: string) {
    super(`AT Protocol sign-in failed: ${code}`);
  }
}

/** Result of a sign-in to the origin server. */
export type OriginAtprotoSignInResult =
  { kind: 'signed-in'; userId: string } | { kind: 'confirm-account'; token: string };

type SignInResponse = {
  status?: string;
  token?: string;
  user?: { id?: string };
  error?: string;
};

async function postSignIn(
  serverOrigin: string,
  body: Record<string, string>,
  browser: boolean
): Promise<SignInResponse> {
  const path = browser ? '/auth/browser/atproto' : '/auth/atproto';
  const response = await fetch(new URL(path, serverOrigin), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(browser ? browserCookieAuthenticationHeaders : {})
    },
    body: JSON.stringify(body),
    credentials: browser ? 'include' : 'omit'
  });
  const result = (await response.json().catch(() => ({}))) as SignInResponse;
  if (!response.ok) throw new AtprotoSignInError(result.error ?? 'provider_failed');
  return result;
}

async function serviceTokenFor(session: OAuthSession, serverOrigin: string): Promise<string> {
  const audience = atprotoServiceAudience(serverOrigin);
  if (!audience) throw new AtprotoSignInError('unsupported_server');
  return atprotoServiceToken(session, audience);
}

/**
 * Sign in to this window's own server with the AT Protocol identity
 * `identifier` (a handle, or empty for the device session). Call this from the
 * user's action: a sign-in to the PDS opens a popup window.
 */
export async function signInToOriginWithAtproto(
  identifier: string,
  options: { admission?: string } = {}
): Promise<OriginAtprotoSignInResult> {
  const session = await ensureAtprotoSession(identifier).catch((error: unknown) => {
    // The OAuth library reports a handle or DID that it cannot resolve this way.
    if (
      error instanceof Error &&
      (error.name === 'HandleResolverError' || error.name === 'IdentityResolverError')
    ) {
      throw new AtprotoSignInError('atproto_identity_not_found');
    }
    throw error;
  });
  const origin = window.location.origin;
  const result = await postSignIn(
    origin,
    {
      token: await serviceTokenFor(session, origin),
      ...(options.admission ? { admission: options.admission } : {})
    },
    true
  );
  if (result.status === 'confirm_account' && result.token) {
    return { kind: 'confirm-account', token: result.token };
  }
  const userId = result.user?.id;
  if (result.status !== 'signed_in' || !userId) throw new AtprotoSignInError('provider_failed');
  await ensureAtprotoLinkRecord({ serverOrigin: origin, userId }, session.did);
  return { kind: 'signed-in', userId };
}

/**
 * Sign in to the server at `serverUrl` with the device's AT Protocol
 * session, or the one for `identifier`. Returns
 * the new bearer session, or `null` when no account there uses this identity
 * yet; the caller then offers that server's own sign-in page.
 */
export async function signInToRemoteWithAtproto(
  serverUrl: string,
  identifier = ''
): Promise<NewBearerSession | null> {
  const serverOrigin = new URL(serverUrl).origin;
  const session = await ensureAtprotoSession(identifier);
  let result: SignInResponse;
  try {
    result = await postSignIn(
      serverOrigin,
      { token: await serviceTokenFor(session, serverOrigin) },
      false
    );
  } catch (error) {
    if (error instanceof AtprotoSignInError && error.code === 'external_identity_unlinked') {
      return null;
    }
    throw error;
  }
  const credentials = directBearerSession(result);
  const userId = result.user?.id;
  if (!credentials || !userId) throw new AtprotoSignInError('provider_failed');
  await ensureAtprotoLinkRecord({ serverOrigin, userId }, session.did);
  return credentials;
}

/**
 * Link the session's AT Protocol identity to the account on `serverOrigin`
 * that `linkToken` names. `StartExternalIdentityLink` issues the token after
 * it checks the user's recent authentication.
 */
export async function linkAtprotoIdentity(
  session: OAuthSession,
  serverOrigin: string,
  linkToken: string
): Promise<void> {
  const result = await postSignIn(
    serverOrigin,
    { token: await serviceTokenFor(session, serverOrigin), link_token: linkToken },
    false
  );
  const userId = result.user?.id;
  if (result.status !== 'linked' || !userId) throw new AtprotoSignInError('provider_failed');
  await ensureAtprotoLinkRecord({ serverOrigin, userId }, session.did);
}

/**
 * Write the account link record for `account` when the device session
 * belongs to `did` and other clients cannot verify the link yet. Without a
 * matching device session, this does nothing.
 */
export async function ensureAtprotoLinkRecord(
  account: ChattoAccountRef,
  did: string
): Promise<void> {
  const session = await atprotoRepoSession(did);
  if (!session) return;
  atprotoLinkVerifier.forget(did, account);
  const verification = await atprotoLinkVerifier.verify(did, account);
  if (verification.status === 'verified') return;
  await writeAccountLinkRecord(session, account);
  atprotoLinkVerifier.forget(did, account);
}

/** Delete the account link record when the device session belongs to `did`. */
export async function removeAtprotoLinkRecord(
  account: ChattoAccountRef,
  did: string
): Promise<void> {
  const session = await atprotoRepoSession(did);
  if (!session) return;
  await deleteAccountLinkRecord(session, account);
  atprotoLinkVerifier.forget(did, account);
}
