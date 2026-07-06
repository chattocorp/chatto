import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  getCurrentUserViaConnectMock,
  clearOriginAuthenticationMock,
  handleAuthenticationRequiredMock,
  clearAuthenticationRequiredMock
} = vi.hoisted(() => ({
  getCurrentUserViaConnectMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn(),
  handleAuthenticationRequiredMock: vi.fn(),
  clearAuthenticationRequiredMock: vi.fn()
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
    get originServer() {
      return { id: 'origin' };
    },
    clearOriginAuthentication: clearOriginAuthenticationMock,
    handleAuthenticationRequired: handleAuthenticationRequiredMock,
    clearAuthenticationRequired: clearAuthenticationRequiredMock
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
    expect(clearAuthenticationRequiredMock).toHaveBeenCalledWith('origin');
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

  it('keeps the cached user and marks reauth required on authentication-required errors', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock
      .mockResolvedValueOnce(user)
      .mockRejectedValueOnce({ message: 'authentication required' });

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual(user);
    expect(handleAuthenticationRequiredMock).toHaveBeenCalledWith('origin');
    expect(clearOriginAuthenticationMock).not.toHaveBeenCalled();
  });

  it('clears origin auth on first-load authentication-required errors', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserViaConnectMock.mockRejectedValueOnce({ message: 'authentication required' });

    expect(await loadCurrentUser()).toBeNull();
    expect(clearOriginAuthenticationMock).toHaveBeenCalledOnce();
    expect(handleAuthenticationRequiredMock).not.toHaveBeenCalled();
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
