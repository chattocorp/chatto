import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CurrentUserState } from './currentUser.svelte';

const { gotoMock, clearCachedUserMock } = vi.hoisted(() => ({
  gotoMock: vi.fn(() => Promise.resolve()),
  clearCachedUserMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({
  goto: gotoMock
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('./loadAuth', () => ({
  clearCachedUser: clearCachedUserMock
}));

/**
 * CurrentUserState class structure tests.
 *
 * Most behavior is exercised end-to-end through `ServerStateStore`, which
 * constructs one instance per registered server. These tests cover the
 * isolated auth-failure contract because it protects against destructive
 * logout regressions.
 */
describe('CurrentUserState', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('{}', { status: 200 })))
    );
    vi.stubGlobal('sessionStorage', {
      setItem: vi.fn(),
      getItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn()
    });
    vi.stubGlobal('window', {
      location: {
        pathname: '/chat/-/overview',
        search: '?tab=profile'
      }
    });
  });

  it('exports the class', () => {
    expect(CurrentUserState).toBeDefined();
    expect(typeof CurrentUserState).toBe('function');
  });

  it('does not revoke the server session by default for cookie auth failures', async () => {
    const state = new CurrentUserState(true);

    await state.handleAuthFailure();

    expect(fetch).not.toHaveBeenCalled();
    expect(clearCachedUserMock).toHaveBeenCalledOnce();
    expect(sessionStorage.setItem).toHaveBeenCalledWith(
      'returnUrl',
      '/chat/-/overview?tab=profile'
    );
    expect(gotoMock).toHaveBeenCalledWith('/', { invalidateAll: true });
  });

  it('revokes the server session when explicitly requested', async () => {
    const state = new CurrentUserState(true);

    await state.handleAuthFailure({ revokeServerSession: true });

    expect(fetch).toHaveBeenCalledWith('/auth/logout', {
      method: 'POST',
      headers: expect.any(Headers)
    });
    expect(gotoMock).toHaveBeenCalledWith('/', { invalidateAll: true });
  });
});
