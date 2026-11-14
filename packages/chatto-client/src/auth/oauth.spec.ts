// @vitest-environment node
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  createOAuthFlow,
  exchangeAuthorizationCode,
  exchangeOAuthCode,
  isExpectedOAuthIssuer,
  isOAuthCallback,
  pkceCodeChallenge,
  randomOAuthValue
} from './oauth.js';

const config = {
  origin: 'https://chat.example',
  authorizeUrl: '/oauth/authorize',
  clientId: 'https://client.example/native.json',
  redirectUri: 'http://127.0.0.1:3333/servers/callback?mode=popup'
};
const credentials = {
  access_token: 'fixture-access',
  refresh_token: 'fixture-refresh',
  expires_in: 300,
  refresh_token_expires_in: 3600
};
function callback(state: string, query = 'code=fixture-code', issuer = config.origin) {
  return `${config.redirectUri}&state=${state}&${query}&iss=${encodeURIComponent(issuer)}`;
}
afterEach(() => vi.unstubAllGlobals());

describe('native browser OAuth', () => {
  it('uses fresh S256 PKCE and state for each attempt', async () => {
    const first = await createOAuthFlow(config);
    const second = await createOAuthFlow(config);
    const url = new URL(first.authorizationUrl);
    expect(first.verifier).not.toBe(second.verifier);
    expect(first.state).not.toBe(second.state);
    expect(first.verifier.length).toBeGreaterThanOrEqual(43);
    expect(url.searchParams.get('code_challenge_method')).toBe('S256');
    const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(first.verifier));
    expect(url.searchParams.get('code_challenge')).toBe(Buffer.from(hash).toString('base64url'));
    expect(url.searchParams.get('redirect_uri')).toBe(config.redirectUri);
  });
  it('accepts the desktop callback and rejects substitution of its custom-scheme host', async () => {
    const flow = await createOAuthFlow({
      ...config,
      clientId: 'chatto://desktop',
      redirectUri: 'chatto://desktop/servers/callback?mode=popup'
    });
    const callback = `${flow.redirectUri}&state=${flow.state}&code=fixture-code&iss=${encodeURIComponent(flow.origin)}`;
    expect(isOAuthCallback(flow, callback)).toBe(true);
    expect(isOAuthCallback(flow, callback.replace('chatto://desktop', 'chatto://other'))).toBe(
      false
    );
    expect(isOAuthCallback(flow, callback.replace('chatto://desktop', 'other://desktop'))).toBe(
      false
    );
  });
  it.each([
    { origin: 'http://remote.example' },
    { origin: 'https://chat.example/path' },
    { authorizeUrl: 'https://other.example/authorize' },
    { authorizeUrl: '' },
    { authorizeUrl: '/authorize#fragment' },
    { authorizeUrl: 'https://user:secret@chat.example/authorize' },
    { redirectUri: 'https://callback.example/' },
    { redirectUri: 'chatto://other/servers/callback?mode=popup' },
    { redirectUri: 'chatto://desktop/other?mode=popup' },
    { redirectUri: 'http://localhost:3333/callback' },
    { redirectUri: 'http://user@127.0.0.1:3333/callback' },
    { redirectUri: 'http://127.0.0.1:3333/callback#bad' },
    { clientId: '' }
  ])('rejects invalid origin or authorization configuration %o', async (patch) => {
    await expect(createOAuthFlow({ ...config, ...patch })).rejects.toThrow();
  });
  it('accepts exact callbacks, a bound issuer, and a declined flow', async () => {
    const flow = await createOAuthFlow(config);
    expect(isOAuthCallback(flow, callback(flow.state))).toBe(true);
    expect(isOAuthCallback(flow, callback(flow.state, 'error=access_denied'))).toBe(true);
    await expect(
      exchangeOAuthCode(flow, callback(flow.state, 'error=access_denied'))
    ).rejects.toThrow('declined');
  });
  it('accepts only the issuer of the server that started the flow', () => {
    expect(isExpectedOAuthIssuer('https://chat.example', 'https://chat.example/')).toBe(true);
    expect(isExpectedOAuthIssuer('https://CHAT.example:443', 'https://chat.example')).toBe(true);
    // A response without an issuer can come from a different server.
    for (const missing of [undefined, null, '']) {
      expect(isExpectedOAuthIssuer(missing, 'https://chat.example')).toBe(false);
    }
    for (const other of ['https://other.example', 'http://chat.example', 'not a URL']) {
      expect(isExpectedOAuthIssuer(other, 'https://chat.example')).toBe(false);
    }
  });
  it('rejects callback substitution and duplicate parameters', async () => {
    const flow = await createOAuthFlow(config);
    for (const url of [
      'invalid',
      callback('wrong-state'),
      callback(flow.state).replace('3333', '3334'),
      callback(flow.state).replace('/servers/callback', '/other'),
      callback(flow.state).replace('mode=popup', 'mode=other'),
      callback(flow.state) + '&mode=popup',
      callback(flow.state) + '&state=other',
      callback(flow.state) + '&code=other',
      callback(flow.state) + '&error=',
      callback(flow.state) + '&error=denied',
      callback(flow.state, 'code=fixture-code', 'https://other.example'),
      callback(flow.state, 'code=fixture-code', ''),
      `${config.redirectUri}&state=${flow.state}&code=fixture-code`,
      callback(flow.state) + '&iss=a&iss=b',
      callback(flow.state) + '#fragment',
      callback(flow.state).replace('127.0.0.1', 'user@127.0.0.1'),
      callback(flow.state, 'error=one&error=two'),
      callback(flow.state, '')
    ])
      expect(isOAuthCallback(flow, url)).toBe(false);
    await expect(exchangeOAuthCode(flow, callback('wrong'))).rejects.toThrow('Invalid');
  });
  it('exchanges only with the bound issuer, verifier, and public client', async () => {
    const flow = await createOAuthFlow(config);
    const fetch = vi.fn().mockResolvedValue(Response.json(credentials));
    vi.stubGlobal('fetch', fetch);
    await expect(exchangeOAuthCode(flow, callback(flow.state))).resolves.toMatchObject({
      token: 'fixture-access',
      refreshToken: 'fixture-refresh',
      oauthClientId: config.clientId
    });
    const [url, options] = fetch.mock.calls[0]!;
    expect(String(url)).toBe('https://chat.example/oauth/token');
    expect(options).toMatchObject({ method: 'POST', credentials: 'omit', redirect: 'error' });
    expect(JSON.parse(options.body)).toEqual({
      grant_type: 'authorization_code',
      code: 'fixture-code',
      code_verifier: flow.verifier,
      redirect_uri: config.redirectUri,
      client_id: config.clientId
    });
  });
  it('makes URL-safe PKCE values and the RFC 7636 example challenge', async () => {
    expect(randomOAuthValue(64)).toMatch(/^[A-Za-z0-9_-]{86}$/);
    expect(randomOAuthValue(32)).not.toBe(randomOAuthValue(32));
    expect(await pkceCodeChallenge('dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk')).toBe(
      'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM'
    );
  });
  it.each([
    {
      user: { id: 'U1', login: 'ada', displayName: 'Ada', avatarUrl: '/a.png' },
      expected: { id: 'U1', login: 'ada', displayName: 'Ada', avatarUrl: '/a.png' }
    },
    {
      user: { id: 7, login: null },
      expected: { id: null, login: null, displayName: null, avatarUrl: null }
    },
    { user: 'invalid', expected: { id: null, login: null, displayName: null, avatarUrl: null } },
    { user: undefined, expected: { id: null, login: null, displayName: null, avatarUrl: null } }
  ])('returns the account summary of the token response %#', async ({ user, expected }) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ ...credentials, user })));
    await expect(
      exchangeAuthorizationCode({
        origin: 'https://chat.example',
        code: 'fixture-code',
        verifier: 'fixture-verifier',
        redirectUri: 'https://app.example/servers/callback?mode=popup',
        clientId: 'https://app.example/oauth/frontend-client-metadata.json'
      })
    ).resolves.toMatchObject({ credentials: { token: 'fixture-access' }, user: expected });
  });
  it.each([
    () => Response.json({ error_description: 'private-provider-data' }, { status: 400 }),
    () => new Response(null),
    () => new Response('not JSON'),
    () => Response.json(null),
    () => Response.json({}),
    () => Response.json({ access_token: 'no-refresh-token' }),
    () => new Response('a'.repeat(64 * 1024 + 1))
  ])('keeps bounded exchange failures private', async (response) => {
    const flow = await createOAuthFlow(config);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response()));
    await expect(exchangeOAuthCode(flow, callback(flow.state))).rejects.toThrow(
      'Could not complete browser sign-in'
    );
  });
  it('passes host cancellation to the exchange', async () => {
    const flow = await createOAuthFlow(config);
    const controller = new AbortController();
    controller.abort();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url, options) => {
        expect(options.signal.aborted).toBe(true);
        throw new Error('private transport detail');
      })
    );
    await expect(
      exchangeOAuthCode(flow, callback(flow.state), { signal: controller.signal })
    ).rejects.toThrow('Could not complete browser sign-in');
  });
});
