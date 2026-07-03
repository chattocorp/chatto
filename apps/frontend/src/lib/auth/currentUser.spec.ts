import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CurrentUserState } from './currentUser.svelte';
import { PresenceStatus } from '$lib/render/types';

const { clearCachedUserMock } = vi.hoisted(() => ({
  clearCachedUserMock: vi.fn()
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

  it('marks auth required without revoking the server session by default', async () => {
    const onAuthenticationRequired = vi.fn();
    const state = new CurrentUserState(true, undefined, undefined, onAuthenticationRequired);

    await state.handleAuthFailure();

    expect(fetch).not.toHaveBeenCalled();
    expect(clearCachedUserMock).not.toHaveBeenCalled();
    expect(onAuthenticationRequired).toHaveBeenCalledOnce();
  });

  it('revokes the server session without marking reauth when explicitly requested', async () => {
    const onAuthenticationRequired = vi.fn();
    const state = new CurrentUserState(true, undefined, undefined, onAuthenticationRequired);
    state.user = {
      id: 'U1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      presenceStatus: PresenceStatus.Online,
      hasVerifiedEmail: true,
      viewerCanDeleteAccount: false,
      hasPassword: true,
      settings: null
    };

    await state.handleAuthFailure({ revokeServerSession: true });

    expect(fetch).toHaveBeenCalledWith('/auth/logout', {
      method: 'POST',
      headers: expect.any(Headers)
    });
    expect(state.user).toBeUndefined();
    expect(clearCachedUserMock).toHaveBeenCalledOnce();
    expect(onAuthenticationRequired).not.toHaveBeenCalled();
  });
});
