/**
 * OAuth Authorization Code with PKCE for Chatto servers. This module has the
 * protocol rules: PKCE values, the issuer check, and the code exchange. The
 * host owns presentation and its client identity. `createOAuthFlow` and
 * `isOAuthCallback` are for native browser hosts.
 */
import { nativeServerOrigin } from './nativeOrigin.js';
import { oauthBearerSession, type NewBearerSession } from './bearerSession.js';

function base64Url(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

/**
 * A cryptographically random URL-safe value of `size` bytes. Use 64 bytes for
 * a PKCE code verifier (RFC 7636) and 32 bytes for a `state` value.
 */
export function randomOAuthValue(size: number): string {
  return base64Url(crypto.getRandomValues(new Uint8Array(size)));
}

/** The S256 PKCE code challenge of a code verifier. */
export async function pkceCodeChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
  return base64Url(new Uint8Array(digest));
}

/** A single in-memory authorization attempt. Never log or persist its verifier or callback. */
export type OAuthFlow = {
  origin: string;
  clientId: string;
  redirectUri: string;
  state: string;
  verifier: string;
  authorizationUrl: string;
};

/** Create an S256 authorization URL for the server's discovered authorization endpoint. */
export async function createOAuthFlow(input: {
  origin: string;
  authorizeUrl: string;
  clientId: string;
  redirectUri: string;
}): Promise<OAuthFlow> {
  const origin = nativeServerOrigin(input.origin).origin;
  const authorize = new URL(input.authorizeUrl, origin);
  const callback = new URL(input.redirectUri);
  if (
    !input.authorizeUrl ||
    authorize.origin !== origin ||
    authorize.username ||
    authorize.password ||
    authorize.hash ||
    callback.username ||
    callback.password ||
    callback.hash ||
    !input.clientId
  )
    throw new Error('Invalid browser sign-in configuration');
  // Permit the built-in desktop route or HTTP on a literal loopback interface.
  if (
    input.redirectUri !== 'chatto://desktop/servers/callback?mode=popup' &&
    (callback.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(callback.hostname))
  )
    throw new Error('A native callback is required');
  const verifier = randomOAuthValue(64);
  const state = randomOAuthValue(32);
  const challenge = await pkceCodeChallenge(verifier);
  for (const [name, value] of Object.entries({
    response_type: 'code',
    client_id: input.clientId,
    redirect_uri: input.redirectUri,
    code_challenge: challenge,
    code_challenge_method: 'S256',
    state
  }))
    authorize.searchParams.set(name, value);
  return {
    origin,
    clientId: input.clientId,
    redirectUri: input.redirectUri,
    state,
    verifier,
    authorizationUrl: authorize.href
  };
}

/**
 * Whether an authorization response comes from the server that the flow
 * started with (RFC 9207). A client that signs in to more than one server must
 * make this check before it sends a code to a token endpoint. Without it, a
 * malicious server can pass the flow to a different server and receive that
 * server's code together with the PKCE verifier.
 *
 * `issuer` is the `iss` parameter of the response. A missing or empty value
 * fails. A response without an issuer can come from a different server, so
 * accepting it would leave accounts on servers that send no issuer open to
 * this attack. A Chatto server sends the origin of its public URL. An older
 * server without a public URL sends an empty issuer.
 */
export function isExpectedOAuthIssuer(
  issuer: string | null | undefined,
  serverOrigin: string
): boolean {
  if (!issuer) return false;
  try {
    return new URL(issuer).origin === new URL(serverOrigin).origin;
  } catch {
    return false;
  }
}

/** Validate the exact callback route, transaction state and issuer before accepting a code. */
export function isOAuthCallback(flow: OAuthFlow, value: string): boolean {
  try {
    const url = new URL(value);
    const expected = new URL(flow.redirectUri);
    if (
      url.protocol !== expected.protocol ||
      url.host !== expected.host ||
      url.origin !== expected.origin ||
      url.pathname !== expected.pathname ||
      url.username ||
      url.password ||
      url.hash ||
      url.searchParams.getAll('state').length !== 1 ||
      url.searchParams.get('state') !== flow.state ||
      url.searchParams.getAll('code').length > 1 ||
      url.searchParams.getAll('error').length > 1 ||
      url.searchParams.getAll('iss').length > 1 ||
      !isExpectedOAuthIssuer(url.searchParams.get('iss'), flow.origin)
    )
      return false;
    for (const [key, val] of expected.searchParams)
      if (url.searchParams.getAll(key).length !== 1 || url.searchParams.get(key) !== val)
        return false;
    const code = url.searchParams.get('code');
    const error = url.searchParams.get('error');
    return (!!code && !url.searchParams.has('error')) || (!!error && !url.searchParams.has('code'));
  } catch {
    return false;
  }
}

/** Account summary that the token response gives for the new session. */
export type OAuthSessionUser = {
  id: string | null;
  login: string | null;
  displayName: string | null;
  avatarUrl: string | null;
};

/** One authorization code and the values that the authorization request bound it to. */
export type AuthorizationCodeExchange = {
  /** Origin of the server that issued the code. */
  origin: string;
  code: string;
  verifier: string;
  redirectUri: string;
  clientId: string;
  signal?: AbortSignal;
};

/**
 * Exchange an authorization code for a renewable bearer session at the
 * server's token endpoint. Check the response issuer with
 * `isExpectedOAuthIssuer` before this call, so the code and the verifier
 * reach only the server that issued the code.
 *
 * The request sends no cookies, follows no redirect, and reads at most 64 KiB.
 * Each failure throws the same error, without server text.
 */
export async function exchangeAuthorizationCode(
  input: AuthorizationCodeExchange
): Promise<{ credentials: NewBearerSession; user: OAuthSessionUser }> {
  const signal = AbortSignal.any([
    AbortSignal.timeout(15_000),
    ...(input.signal ? [input.signal] : [])
  ]);
  try {
    const response = await fetch(new URL('/oauth/token', input.origin), {
      method: 'POST',
      credentials: 'omit',
      redirect: 'error',
      signal,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        grant_type: 'authorization_code',
        code: input.code,
        code_verifier: input.verifier,
        redirect_uri: input.redirectUri,
        client_id: input.clientId
      })
    });
    if (!response.ok || !response.body) {
      await response.body?.cancel();
      throw new Error();
    }
    const reader = response.body.getReader();
    let size = 0;
    let text = '';
    const decoder = new TextDecoder();
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        size += value.byteLength;
        if (size > 64 * 1024) throw new Error();
        text += decoder.decode(value, { stream: true });
      }
      text += decoder.decode();
    } finally {
      await reader.cancel().catch(() => {});
      reader.releaseLock();
    }
    const body: unknown = JSON.parse(text);
    if (!body || typeof body !== 'object' || !('access_token' in body)) throw new Error();
    const credentials = oauthBearerSession(body, input.clientId);
    if (!credentials) throw new Error();
    const user: Record<string, unknown> =
      'user' in body && body.user && typeof body.user === 'object' ? { ...body.user } : {};
    const stringOrNull = (value: unknown) => (typeof value === 'string' ? value : null);
    return {
      credentials,
      user: {
        id: stringOrNull(user.id),
        login: stringOrNull(user.login),
        displayName: stringOrNull(user.displayName),
        avatarUrl: stringOrNull(user.avatarUrl)
      }
    };
  } catch {
    throw new Error('Could not complete browser sign-in');
  }
}

/** Exchange one validated callback. Failures never expose server text. */
export async function exchangeOAuthCode(
  flow: OAuthFlow,
  callback: string,
  options: { signal?: AbortSignal } = {}
): Promise<NewBearerSession> {
  if (!isOAuthCallback(flow, callback)) throw new Error('Invalid browser sign-in callback');
  const code = new URL(callback).searchParams.get('code');
  if (!code) throw new Error('Browser sign-in was declined');
  const { credentials } = await exchangeAuthorizationCode({
    origin: flow.origin,
    code,
    verifier: flow.verifier,
    redirectUri: flow.redirectUri,
    clientId: flow.clientId,
    signal: options.signal
  });
  return credentials;
}
