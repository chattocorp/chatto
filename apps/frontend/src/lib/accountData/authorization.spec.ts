import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { getClientConfigurationMock, navigateMock, closeMock, openOAuthPopupMock } = vi.hoisted(
  () => ({
    getClientConfigurationMock: vi.fn(),
    navigateMock: vi.fn(),
    closeMock: vi.fn(),
    openOAuthPopupMock: vi.fn(() => ({
      response: Promise.resolve({ state: 'state', code: 'code' }),
      navigate: navigateMock,
      close: closeMock
    }))
  })
);

vi.mock('$lib/clientConfig', () => ({ getClientConfiguration: getClientConfigurationMock }));
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
    getClientConfigurationMock.mockResolvedValue({
      version: 1,
      authling: {
        issuer: 'https://id.example',
        clientId: 'https://chat.example/oauth/frontend-client-metadata.json'
      }
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
          userinfo_endpoint: 'https://id.example/oauth/userinfo',
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
      )
      .mockResolvedValueOnce(Response.json({ sub: 'account-123' }));
    vi.stubGlobal('fetch', fetchMock);

    const { authorizeAccountData } = await import('./authorization');
    const authorizationPromise = authorizeAccountData();
    expect(openOAuthPopupMock).toHaveBeenCalledOnce();
    const authorization = await authorizationPromise;

    expect(navigateMock).toHaveBeenCalledOnce();
    const authorizeURL = new URL(navigateMock.mock.calls[0]![0]);
    expect(authorizeURL.searchParams.get('client_id')).toBe(
      'https://chat.example/oauth/frontend-client-metadata.json'
    );
    expect(authorizeURL.searchParams.get('redirect_uri')).toBe(
      'https://chat.example/servers/callback?mode=authling-account-data'
    );
    expect(authorizeURL.searchParams.get('scope')).toBe('openid account_data');

    const tokenRequest = fetchMock.mock.calls[1]![1] as RequestInit;
    expect(tokenRequest.method).toBe('POST');
    expect(String(tokenRequest.body)).toContain(
      'client_id=https%3A%2F%2Fchat.example%2Foauth%2Ffrontend-client-metadata.json'
    );
    expect(authorization).toEqual(
      expect.objectContaining({
        accessToken: 'access-token',
        issuer: 'https://id.example',
        clientId: 'https://chat.example/oauth/frontend-client-metadata.json',
        accountId: 'account-123',
        providerLabel: 'Authling'
      })
    );
  });

  it('rejects Authling when it does not advertise account data', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        Response.json({
          issuer: 'https://id.example',
          authorization_endpoint: 'https://id.example/oauth/authorize',
          token_endpoint: 'https://id.example/oauth/token',
          userinfo_endpoint: 'https://id.example/oauth/userinfo',
          scopes_supported: ['openid'],
          code_challenge_methods_supported: ['S256'],
          token_endpoint_auth_methods_supported: ['none']
        })
      )
    );

    const { authorizeAccountData } = await import('./authorization');
    await expect(authorizeAccountData()).rejects.toThrow(
      'This OIDC provider does not support account data'
    );
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('rejects account-data authorization when the client does not select Authling', async () => {
    getClientConfigurationMock.mockResolvedValue({ version: 1, authling: null });
    vi.stubGlobal('fetch', vi.fn());

    const { authorizeAccountData } = await import('./authorization');
    await expect(authorizeAccountData()).rejects.toThrow(
      'This client does not configure an Authling issuer'
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
          userinfo_endpoint: 'https://id.example/oauth/userinfo',
          scopes_supported: ['openid', 'account_data'],
          code_challenge_methods_supported: ['S256'],
          token_endpoint_auth_methods_supported: ['none']
        })
      )
    );

    const { authorizeAccountData } = await import('./authorization');
    await expect(authorizeAccountData()).rejects.toThrow(
      'OIDC discovery returned a cross-origin endpoint'
    );
  });
});
