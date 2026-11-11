/**
 * The device's AT Protocol session (FDR-056).
 *
 * The frontend signs in to the user's PDS itself, with its own OAuth client.
 * It asks for the identity, for write access to Chatto account link records,
 * and for `run.chatto.signIn` service tokens. Chatto servers never receive the
 * OAuth tokens; they accept short-lived service tokens instead (ADR-123). One
 * session per device signs the user in to every Chatto server.
 *
 * The OAuth library loads only when the user connects an identity or a
 * session was stored before, so other users do not download it.
 */

import type { BrowserOAuthClient, OAuthSession } from '@atproto/oauth-client-browser';
import {
  ATPROTO_SIGN_IN_METHOD,
  createAtprotoLinkVerifier,
  resolveAtprotoHandle,
  resolveAtprotoIdentity
} from '@chatto/client/atproto/accountLink';

/** Scope of the frontend client. It must match the published client metadata. */
const ATPROTO_FRONTEND_SCOPE = 'atproto repo:run.chatto.accountLink rpc:run.chatto.signIn?aud=*';
/** Lifetime of sign-in service tokens; servers reject longer ones. */
const SERVICE_TOKEN_LIFETIME_SECONDS = 60;
const CLIENT_METADATA_PATH = '/oauth/atproto-client-metadata.json';
export const ATPROTO_CALLBACK_PATH = '/atproto/callback';
/** Authorization server for users who start without a handle. */
const DEFAULT_AUTH_SERVER = 'https://bsky.social';
/** Key that the OAuth library sets while a session exists. */
const LIBRARY_SESSION_KEY = '@@atproto/oauth-client-browser(sub)';

/** The signed-in AT Protocol identity of this device. */
export type AtprotoAccount = {
  did: string;
  /** Handle from the DID document; `null` when it cannot be resolved. */
  handle: string | null;
};

/** Verifies other users' published DIDs for every server of this device. */
export const atprotoLinkVerifier = createAtprotoLinkVerifier();

let account = $state<AtprotoAccount | null>(null);
let clientPromise: Promise<BrowserOAuthClient> | null = null;
let session: OAuthSession | null = null;
/** The first restore. Later calls share it; sign-in and sign-out update `account`. */
let restorePromise: Promise<AtprotoAccount | null> | null = null;

/** The current AT Protocol account, or `null` when this device has none. */
export function atprotoAccount(): AtprotoAccount | null {
  return account;
}

/** Redirect URI of the loopback client that plain-HTTP development origins use. */
function loopbackRedirectUri(): `http://127.0.0.1:${string}` {
  return `http://127.0.0.1:${window.location.port}${ATPROTO_CALLBACK_PATH}`;
}

/** Load the OAuth client for this origin. */
export function atprotoOAuthClient(): Promise<BrowserOAuthClient> {
  clientPromise ??= (async () => {
    const { BrowserOAuthClient } = await import('@atproto/oauth-client-browser');
    // Resolve handles like the verifier, without a fixed third-party resolver.
    const handleResolver = {
      resolve: async (handle: string) =>
        (await resolveAtprotoHandle(handle)) as `did:plc:${string}` | `did:web:${string}` | null
    };
    if (window.location.protocol === 'https:') {
      return BrowserOAuthClient.load({
        clientId: `${window.location.origin}${CLIENT_METADATA_PATH}`,
        handleResolver
      });
    }
    // Development servers use AT Protocol's loopback client. It returns to
    // 127.0.0.1; the Chatto server forwards that callback to its configured
    // origin, where this window keeps the sign-in state. The query response
    // mode makes the callback parameters visible to the server.
    const redirectUri = loopbackRedirectUri();
    const params = `scope=${encodeURIComponent(ATPROTO_FRONTEND_SCOPE)}&redirect_uri=${encodeURIComponent(redirectUri)}`;
    return BrowserOAuthClient.load({
      clientId: `http://localhost?${params}`,
      handleResolver,
      responseMode: 'query'
    });
  })();
  clientPromise.catch(() => (clientPromise = null));
  return clientPromise;
}

async function accountFor(oauthSession: OAuthSession): Promise<AtprotoAccount> {
  const identity = await resolveAtprotoIdentity(oauthSession.did).catch(() => null);
  const handle =
    identity?.handle && (await resolveAtprotoHandle(identity.handle)) === oauthSession.did
      ? identity.handle
      : null;
  return { did: oauthSession.did, handle };
}

/**
 * Restore a stored session once per page. Without a stored session, this
 * returns `null` without loading the OAuth library.
 */
export function restoreAtprotoAccount(): Promise<AtprotoAccount | null> {
  if (typeof window === 'undefined' || !window.localStorage?.getItem(LIBRARY_SESSION_KEY)) {
    return Promise.resolve(account);
  }
  restorePromise ??= (async () => {
    try {
      const client = await atprotoOAuthClient();
      const restored = await client.initRestore();
      // A sign-in that finished meanwhile wins over the stored session. A
      // session from an older client version can lack a permission; the next
      // sign-in replaces it.
      if (!session && restored && (await hasFrontendScope(restored.session))) {
        session = restored.session;
        account = await accountFor(restored.session);
      }
    } catch {
      // An unusable stored session counts as no session.
    }
    return account;
  })();
  return restorePromise.then(() => account);
}

/**
 * Sign in to the user's PDS in a popup window. Call this from the user's
 * action. `identifier` is a handle or DID; without it, the user enters the
 * handle on the default authorization server.
 */
export async function connectAtprotoAccount(identifier: string): Promise<AtprotoAccount> {
  const client = await atprotoOAuthClient();
  const input = identifier.trim().replace(/^@/, '') || DEFAULT_AUTH_SERVER;
  session = await client.signInPopup(input, { scope: ATPROTO_FRONTEND_SCOPE });
  account = await accountFor(session);
  return account;
}

async function hasFrontendScope(oauthSession: OAuthSession): Promise<boolean> {
  const granted = new Set((await oauthSession.getTokenInfo(false)).scope.split(' '));
  return ATPROTO_FRONTEND_SCOPE.split(' ').every((scope) => granted.has(scope));
}

/**
 * The device session for `identifier`, a handle or DID. An empty identifier
 * accepts the current session. Without a matching session, this signs in
 * through a popup window, so call it from the user's action.
 */
export async function ensureAtprotoSession(identifier: string): Promise<OAuthSession> {
  await restoreAtprotoAccount();
  const wanted = identifier.trim().replace(/^@/, '').toLowerCase();
  if (session && (!wanted || wanted === account?.handle || wanted === session.did)) return session;
  await connectAtprotoAccount(wanted);
  if (!session) throw new Error('The AT Protocol session is not available.');
  return session;
}

/**
 * Ask the PDS for a short-lived token that signs the user in to the Chatto
 * server with service DID reference `audience`.
 */
export async function atprotoServiceToken(
  oauthSession: OAuthSession,
  audience: string
): Promise<string> {
  const exp = Math.floor(Date.now() / 1000) + SERVICE_TOKEN_LIFETIME_SECONDS;
  const query = `aud=${encodeURIComponent(audience)}&lxm=${ATPROTO_SIGN_IN_METHOD}&exp=${exp}`;
  const response = await oauthSession.fetchHandler(
    `/xrpc/com.atproto.server.getServiceAuth?${query}`,
    { method: 'GET' }
  );
  const body: unknown = await response.json().catch(() => null);
  const token =
    body && typeof body === 'object' && 'token' in body && typeof body.token === 'string'
      ? body.token
      : null;
  if (!response.ok || !token)
    throw new Error(`The PDS issued no sign-in token (HTTP ${response.status}).`);
  return token;
}

/** Sign out of the PDS and revoke the session's tokens. */
export async function disconnectAtprotoAccount(): Promise<void> {
  const current = session;
  session = null;
  account = null;
  if (current) await current.signOut().catch(() => {});
}

/**
 * The repository session for `did`. Returns `null` when this device has no
 * session for that DID; the caller then asks the user to connect it.
 */
export async function atprotoRepoSession(did: string): Promise<OAuthSession | null> {
  if (!session) await restoreAtprotoAccount();
  return session?.did === did ? session : null;
}

/**
 * Complete a sign-in on the callback route. The result goes to the opening
 * window, and the popup tries to close itself. Rejects when the sign-in failed
 * and no opening window received the result.
 */
export async function completeAtprotoCallback(): Promise<void> {
  const [client, { LoginContinuedInParentWindowError }] = await Promise.all([
    atprotoOAuthClient(),
    import('@atproto/oauth-client-browser')
  ]);
  try {
    // A forwarded loopback callback arrives on another origin than its
    // redirect URI, so name the URI that the authorization server used.
    await client.initCallback(
      client.readCallbackParams(),
      window.location.protocol === 'https:' ? client.findRedirectUrl() : loopbackRedirectUri()
    );
  } catch (error) {
    if (error instanceof LoginContinuedInParentWindowError) return;
    throw error;
  }
}
