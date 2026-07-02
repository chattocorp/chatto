import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getCurrentUserViaConnectMock, clearOriginAuthenticationMock } = vi.hoisted(() => ({
  getCurrentUserViaConnectMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn()
}));

vi.mock('$app/environment', () => ({
  browser: true
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$lib/api-client/viewer', () => ({
  getCurrentUserViaConnect: getCurrentUserViaConnectMock
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    originClient: {
      connectBaseUrl: '/api/connect',
      bearerToken: null
    }
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    clearOriginAuthentication: clearOriginAuthenticationMock
  }
}));

const user = {
  id: 'U1',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  presenceStatus: 'ONLINE',
  hasVerifiedEmail: true,
  settings: { timezone: 'UTC', timeFormat: '24h' }
};

async function loadModule() {
  vi.resetModules();
  return import('./loadAuth');
}

describe('loadCurrentUser', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('refreshes from the server on each call', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock
      .mockResolvedValueOnce(user)
      .mockResolvedValueOnce({ ...user, displayName: 'Alice Fresh' });

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual({ ...user, displayName: 'Alice Fresh' });
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledTimes(2);
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledWith({
      baseUrl: '/api/connect',
      bearerToken: null
    });
  });

  it('keeps the cached user when a later refresh errors', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock
      .mockResolvedValueOnce(user)
      .mockRejectedValueOnce(new Error('not found'))
      .mockRejectedValueOnce(new Error('still not found'));

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual(user);
  });

  it('clears the cached user on authentication-required errors', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock
      .mockResolvedValueOnce(user)
      .mockRejectedValueOnce({ message: 'authentication required' });

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toBeNull();
    expect(clearOriginAuthenticationMock).toHaveBeenCalledOnce();
  });

  it('returns null when the first load cannot determine a user', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock.mockRejectedValue(new Error('unreachable'));

    expect(await loadCurrentUser()).toBeNull();
  });

  it('does not clear origin auth for transient errors after retry', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce(user);

    expect(await loadCurrentUser()).toEqual(user);
    expect(clearOriginAuthenticationMock).not.toHaveBeenCalled();
  });
});
