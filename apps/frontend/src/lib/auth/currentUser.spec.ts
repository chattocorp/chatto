import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CurrentUserState, type CurrentUser } from './currentUser.svelte';
vi.mock('./originViewer', () => ({ getOriginViewer: vi.fn() }));

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

  it('shares an in-flight viewer request between concurrent callers', async () => {
    let resolveViewer!: (user: CurrentUser) => void;
    const viewerRequest = new Promise<CurrentUser>((resolve) => {
      resolveViewer = resolve;
    });
    const loadViewer = vi.fn(() => viewerRequest);
    const state = new CurrentUserState(
      false,
      { serverId: 'remote', baseUrl: 'https://remote.example.test', bearerToken: 'token' },
      loadViewer
    );

    const first = state.load();
    const second = state.load();

    expect(loadViewer).toHaveBeenCalledOnce();
    resolveViewer({
      id: 'U1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      presenceStatus: PresenceStatus.ONLINE,
      hasVerifiedEmail: true,
      viewerCanDeleteAccount: false,
      hasPassword: true,
      settings: null
    });
    await Promise.all([first, second]);

    expect(state.user?.id).toBe('U1');
    expect(state.verifiedUserId).toBe('U1');
    expect(state.loading).toBe(false);
  });

  it('lets the owner check an account change before publishing it', async () => {
    const previousViewer = { id: 'U1', login: 'alice', displayName: 'Alice' } as CurrentUser;
    const otherViewer = { id: 'U2', login: 'bob', displayName: 'Bob' } as CurrentUser;
    const loadViewer = vi.fn().mockResolvedValue(otherViewer);
    const onLoaded = vi.fn();
    const state = new CurrentUserState(
      true,
      {
        serverId: 'origin',
        baseUrl: 'https://chat.example.test',
        bearerToken: null
      },
      loadViewer,
      undefined,
      onLoaded
    );
    state.user = previousViewer;

    await state.load();

    expect(onLoaded).toHaveBeenCalledWith(otherViewer);
    expect(state.user).toBe(previousViewer);
    expect(state.verifiedUserId).toBeNull();
  });

  it('does not verify a previous viewer when a network request fails', async () => {
    const previousViewer = { id: 'U1', login: 'alice', displayName: 'Alice' } as CurrentUser;
    const loadViewer = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(previousViewer);
    const state = new CurrentUserState(
      false,
      {
        serverId: 'remote',
        baseUrl: 'https://remote.example.test',
        bearerToken: 'token'
      },
      loadViewer
    );
    state.user = previousViewer;
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      await state.load();
      expect(state.user).toBe(previousViewer);
      expect(state.verifiedUserId).toBeNull();
      await state.load();
      expect(state.verifiedUserId).toBe('U1');
    } finally {
      consoleError.mockRestore();
    }
  });

  it.each(['reset', 'accept'] as const)('discards a late response after %s', async (boundary) => {
    let finish!: (user: CurrentUser) => void;
    const request = new Promise<CurrentUser>((resolve) => {
      finish = resolve;
    });
    const state = new CurrentUserState(
      false,
      { baseUrl: '/api/connect', bearerToken: null },
      () => request
    );
    const stale = { id: 'U1', login: 'old' } as CurrentUser;
    const fresh = { id: 'U2', login: 'new' } as CurrentUser;
    const pending = state.load();
    if (boundary === 'reset') state.reset();
    else state.accept(fresh);
    finish(stale);
    await pending;
    expect(state.user).toBe(boundary === 'reset' ? undefined : fresh);
  });

  it('retains complete account data on failure and revokes verification on rejection', async () => {
    const { Code, ConnectError } = await import('@connectrpc/connect');
    const account = { id: 'U1', login: 'alice' } as CurrentUser;
    const loader = vi
      .fn()
      .mockResolvedValueOnce(account)
      .mockRejectedValueOnce(new Error('offline'))
      .mockRejectedValueOnce(new ConnectError('rejected', Code.Unauthenticated));
    const rejected = vi.fn();
    const state = new CurrentUserState(
      false,
      { baseUrl: '/api/connect', bearerToken: null },
      loader,
      rejected
    );
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      await state.load();
      await state.load();
      expect(state.user).toBe(account);
      expect(state.verifiedUserId).toBe('U1');
      await state.load();
      expect(state.user).toBe(account);
      expect(state.verifiedUserId).toBeNull();
      expect(rejected).toHaveBeenCalledOnce();
    } finally {
      error.mockRestore();
    }
  });

  it('marks auth required without revoking the server session by default', async () => {
    const onAuthenticationRequired = vi.fn();
    const state = new CurrentUserState(true, undefined, undefined, onAuthenticationRequired);
    state.verifiedUserId = 'U1';

    await state.handleAuthFailure();

    expect(state.verifiedUserId).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
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
      presenceStatus: PresenceStatus.ONLINE,
      hasVerifiedEmail: true,
      viewerCanDeleteAccount: false,
      hasPassword: true,
      settings: null
    };

    await state.handleAuthFailure({ revokeServerSession: true });

    expect(fetch).toHaveBeenCalledWith('/auth/browser/logout', {
      method: 'POST',
      headers: expect.any(Headers),
      body: '{}'
    });
    const headers = vi.mocked(fetch).mock.calls[0]?.[1]?.headers as Headers;
    expect(headers.get('Content-Type')).toBe('application/json');
    expect(headers.get('X-Chatto-Authentication-Mode')).toBe('cookie');
    expect(state.user).toBeUndefined();
    expect(onAuthenticationRequired).not.toHaveBeenCalled();
  });
});
