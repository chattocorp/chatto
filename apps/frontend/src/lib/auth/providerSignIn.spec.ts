import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  confirmProviderAccount,
  openProviderSignIn,
  ProviderSignInError,
  providerLoginUrl,
  verifyProviderSignIn
} from './providerSignIn';

const mocks = vi.hoisted(() => ({
  open: vi.fn(),
  getCurrentUser: vi.fn(),
  goto: vi.fn(),
  saveReturnUrl: vi.fn()
}));
vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('./returnNavigation', () => ({ saveReturnUrl: mocks.saveReturnUrl }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/oauth/pkce', () => ({ generateState: () => 'transaction-state' }));
vi.mock('$lib/oauth/popup', () => ({ openOAuthPopup: mocks.open }));
vi.mock('@chatto/client/api/viewer', () => ({ getCurrentUserViaConnect: mocks.getCurrentUser }));

describe('origin provider sign-in', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('window', { location: { origin: 'https://chat.example' } });
  });
  afterEach(() => vi.unstubAllGlobals());

  it('adds a login hint only when one is given', () => {
    expect(providerLoginUrl('/auth/providers/bsky')).toBe('/auth/providers/bsky');
    expect(providerLoginUrl('/auth/providers/bsky', 'alice.example')).toBe(
      '/auth/providers/bsky?login_hint=alice.example'
    );
    expect(providerLoginUrl('https://remote.example/auth/providers/bsky', 'alice.example')).toBe(
      'https://remote.example/auth/providers/bsky?login_hint=alice.example'
    );
  });

  it('opens synchronously and returns to a transaction-bound local callback', () => {
    const popup = { navigate: vi.fn(), close: vi.fn(), response: Promise.resolve({}) };
    mocks.open.mockReturnValue(popup);
    expect(openProviderSignIn('/auth/providers/company')).toBe(popup);
    expect(mocks.open).toHaveBeenCalledWith('transaction-state');
    const url = new URL(popup.navigate.mock.calls[0][0]);
    expect(url.origin + url.pathname).toBe('https://chat.example/auth/providers/company');
    expect(url.searchParams.get('redirect')).toBe(
      '/servers/callback?mode=provider&state=transaction-state'
    );
  });

  it.each([null, { id: 'signed-in-user' }])(
    'verifies the cookie session after completion: %j',
    async (user) => {
      if (user) mocks.getCurrentUser.mockResolvedValue(user);
      else mocks.getCurrentUser.mockRejectedValue(new Error('Unauthenticated'));
      const popup = {
        navigate: vi.fn(),
        close: vi.fn(),
        response: Promise.resolve({
          type: 'chatto:oauth-popup-response' as const,
          state: 'state',
          completed: true
        })
      };
      if (user) await expect(verifyProviderSignIn(popup)).resolves.toEqual({ kind: 'signed-in' });
      else await expect(verifyProviderSignIn(popup)).rejects.toThrow();
      expect(mocks.getCurrentUser).toHaveBeenCalledWith({
        baseUrl: '/api/connect',
        bearerToken: null
      });
    }
  );

  it('hands a new-account confirmation to the opening window', async () => {
    const response = Promise.resolve({
      type: 'chatto:oauth-popup-response' as const,
      state: 'state',
      externalIdentityToken: 'tok'
    });
    await expect(verifyProviderSignIn({ response })).resolves.toEqual({
      kind: 'confirm-account',
      token: 'tok'
    });
    expect(mocks.getCurrentUser).not.toHaveBeenCalled();

    await confirmProviderAccount('tok', '/chat/room');
    expect(mocks.saveReturnUrl).toHaveBeenCalledWith('/chat/room');
    expect(mocks.goto).toHaveBeenCalledWith('/sso/confirm?token=tok');

    mocks.saveReturnUrl.mockClear();
    await confirmProviderAccount('tok', '/');
    await confirmProviderAccount('tok', '/chat');
    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });

  it('reports the server error code of a provider popup', async () => {
    const response = Promise.resolve({
      type: 'chatto:oauth-popup-response' as const,
      state: 'state',
      error: 'external_identity_unlinked'
    });
    const result = verifyProviderSignIn({ response });
    await expect(result).rejects.toBeInstanceOf(ProviderSignInError);
    await expect(result).rejects.toMatchObject({ code: 'external_identity_unlinked' });
  });

  it('does not treat an OAuth code as cookie sign-in completion', async () => {
    await expect(
      verifyProviderSignIn({
        response: Promise.resolve({
          type: 'chatto:oauth-popup-response',
          state: 'state',
          code: 'code'
        })
      })
    ).rejects.toThrow();
    expect(mocks.getCurrentUser).not.toHaveBeenCalled();
  });
});
