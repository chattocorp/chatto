import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { openProviderSignIn, verifyProviderSignIn } from './providerSignIn';

const mocks = vi.hoisted(() => ({
  open: vi.fn(),
  getCurrentUser: vi.fn()
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/oauth/pkce', () => ({ generateState: () => 'transaction-state' }));
vi.mock('$lib/oauth/popup', () => ({ openOAuthPopup: mocks.open }));
vi.mock('$lib/api-client/viewer', () => ({ getCurrentUserViaConnect: mocks.getCurrentUser }));

describe('origin provider sign-in', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('window', { location: { origin: 'https://chat.example' } });
  });
  afterEach(() => vi.unstubAllGlobals());

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
      if (user) await expect(verifyProviderSignIn(popup)).resolves.toBeUndefined();
      else await expect(verifyProviderSignIn(popup)).rejects.toThrow();
      expect(mocks.getCurrentUser).toHaveBeenCalledWith({
        baseUrl: '/api/connect',
        bearerToken: null
      });
    }
  );

  it('does not treat an OAuth code as cookie sign-in completion', async () => {
    await expect(
      verifyProviderSignIn({
        navigate: vi.fn(),
        close: vi.fn(),
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
