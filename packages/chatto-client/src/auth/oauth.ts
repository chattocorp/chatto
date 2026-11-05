/** PKCE protocol for native browser hosts; presentation and client identity belong to the host. */
import { nativeServerOrigin } from './nativeOrigin.js';
import { oauthBearerSession, type NewBearerSession } from './bearerSession.js';

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
  const random = (size: number) => {
    const bytes = crypto.getRandomValues(new Uint8Array(size));
    return btoa(String.fromCharCode(...bytes))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '');
  };
  const verifier = random(64);
  const state = random(32);
  const digest = new Uint8Array(
    await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))
  );
  const challenge = btoa(String.fromCharCode(...digest))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
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

/** Validate the exact callback route, transaction state and optional issuer before accepting a code. */
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
      (url.searchParams.has('iss') && url.searchParams.get('iss') !== flow.origin)
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

/** Exchange one validated callback. The bounded response and private failures never expose server text. */
export async function exchangeOAuthCode(
  flow: OAuthFlow,
  callback: string,
  options: { signal?: AbortSignal } = {}
): Promise<NewBearerSession> {
  if (!isOAuthCallback(flow, callback)) throw new Error('Invalid browser sign-in callback');
  const code = new URL(callback).searchParams.get('code');
  if (!code) throw new Error('Browser sign-in was declined');
  const signal = AbortSignal.any([
    AbortSignal.timeout(15_000),
    ...(options.signal ? [options.signal] : [])
  ]);
  try {
    const response = await fetch(new URL('/oauth/token', flow.origin), {
      method: 'POST',
      credentials: 'omit',
      redirect: 'error',
      signal,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        grant_type: 'authorization_code',
        code,
        code_verifier: flow.verifier,
        redirect_uri: flow.redirectUri,
        client_id: flow.clientId
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
    const credentials = oauthBearerSession(body, flow.clientId);
    if (!credentials) throw new Error();
    return credentials;
  } catch {
    throw new Error('Could not complete browser sign-in');
  }
}
