import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { getPublicServerInfoMock, navigateMock, closeMock, openOAuthPopupMock } = vi.hoisted(() => ({
  getPublicServerInfoMock: vi.fn(),
  navigateMock: vi.fn(),
  closeMock: vi.fn(),
  openOAuthPopupMock: vi.fn(() => ({
    response: Promise.resolve({ state: 'state', code: 'code' }),
    navigate: navigateMock,
    close: closeMock
  }))
}));

vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo: getPublicServerInfoMock }));
vi.mock('$lib/oauth/pkce', () => ({
  generateCodeChallenge: vi.fn(async () => 'challenge'),
  generateCodeVerifier: vi.fn(() => 'verifier'),
  generateState: vi.fn(() => 'state')
}));
vi.mock('$lib/oauth/popup', () => ({
  OAuthPopupError: class extends Error {},
  openOAuthPopup: openOAuthPopupMock
}));

describe('Authling account-data authorization', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('window', { location: { origin: 'https://chat.example' } });
    getPublicServerInfoMock.mockResolvedValue({
      authProviders: [
        {
          id: 'authling',
          type: 'oidc',
          label: 'Authling',
          loginUrl: '/auth/providers/authling',
          issuerUrl: 'https://id.example'
        }
      ]
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it('uses the origin CIMD identity and requests only OIDC account-data access', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          issuer: 'https://id.example',
          authorization_endpoint: 'https://id.example/oauth/authorize',
          token_endpoint: 'https://id.example/oauth/token',
          scopes_supported: ['openid', 'account_data'],
          code_challenge_methods_supported: ['S256'],
          token_endpoint_auth_methods_supported: ['none']
        })
      )
      .mockResolvedValueOnce(
        Response.json({
          access_token: 'access-token',
          expires_in: 300,
          scope: 'openid account_data'
        })
      );
    vi.stubGlobal('fetch', fetchMock);

    const { authorizeAccountData } = await import('./authorization');
    const authorizationPromise = authorizeAccountData();
    expect(openOAuthPopupMock).toHaveBeenCalledOnce();
    const authorization = await authorizationPromise;

    expect(navigateMock).toHaveBeenCalledOnce();
    const authorizeURL = new URL(navigateMock.mock.calls[0]![0]);
    expect(authorizeURL.searchParams.get('client_id')).toBe(
      'https://chat.example/oauth/client-metadata.json'
    );
    expect(authorizeURL.searchParams.get('redirect_uri')).toBe(
      'https://chat.example/servers/callback?mode=authling-account-data'
    );
    expect(authorizeURL.searchParams.get('scope')).toBe('openid account_data');

    const tokenRequest = fetchMock.mock.calls[1]![1] as RequestInit;
    expect(tokenRequest.method).toBe('POST');
    expect(String(tokenRequest.body)).toContain(
      'client_id=https%3A%2F%2Fchat.example%2Foauth%2Fclient-metadata.json'
    );
    expect(authorization).toEqual(
      expect.objectContaining({
        accessToken: 'access-token',
        issuer: 'https://id.example',
        providerLabel: 'Authling'
      })
    );
  });

  it('rejects an OIDC provider that does not advertise account data', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        Response.json({
          issuer: 'https://id.example',
          authorization_endpoint: 'https://id.example/oauth/authorize',
          token_endpoint: 'https://id.example/oauth/token',
          scopes_supported: ['openid'],
          code_challenge_methods_supported: ['S256'],
          token_endpoint_auth_methods_supported: ['none']
        })
      )
    );

    const { authorizeAccountData } = await import('./authorization');
    await expect(authorizeAccountData()).rejects.toThrow(
      'No configured OIDC provider supports account data'
    );
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('rejects discovery endpoints on another origin', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        Response.json({
          issuer: 'https://id.example',
          authorization_endpoint: 'https://evil.example/oauth/authorize',
          token_endpoint: 'https://id.example/oauth/token',
          scopes_supported: ['openid', 'account_data'],
          code_challenge_methods_supported: ['S256'],
          token_endpoint_auth_methods_supported: ['none']
        })
      )
    );

    const { authorizeAccountData } = await import('./authorization');
    await expect(authorizeAccountData()).rejects.toThrow(
      'No configured OIDC provider supports account data'
    );
  });
});
