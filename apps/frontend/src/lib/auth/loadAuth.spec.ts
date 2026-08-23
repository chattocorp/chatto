import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';

const {
  getCurrentUserViaConnectMock,
  clearOriginAuthenticationMock,
  handleAuthenticationRequiredMock,
  clearAuthenticationRequiredMock,
  authenticateOriginBearerMock,
  authenticateOriginCookieMock,
  originState
} = vi.hoisted(() => ({
  getCurrentUserViaConnectMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn(),
  handleAuthenticationRequiredMock: vi.fn(),
  clearAuthenticationRequiredMock: vi.fn(),
  authenticateOriginBearerMock: vi.fn(),
  authenticateOriginCookieMock: vi.fn(),
  originState: { token: null as string | null }
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
    originConnectBaseUrl: '/api/connect'
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get originServer() {
      return { id: 'origin', token: originState.token };
    },
    authenticateOriginBearer: authenticateOriginBearerMock,
    authenticateOriginCookie: authenticateOriginCookieMock,
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

const pendingCredentials = {
  token: 'new-origin-token',
  refreshToken: 'new-origin-refresh-token',
  expiresIn: 900,
  refreshTokenExpiresIn: 7776000,
  oauthClientId: null
};

async function loadModule() {
  vi.resetModules();
  return import('./loadAuth');
}

describe('loadCurrentUser', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    originState.token = null;
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
    expect(authenticateOriginCookieMock).toHaveBeenCalledTimes(2);
  });

  it('discards a legacy origin bearer after cookie authentication succeeds', async () => {
    originState.token = 'legacy-origin-token';
    getCurrentUserViaConnectMock.mockResolvedValueOnce(user);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledOnce();
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledWith(
      expect.objectContaining({ bearerToken: null })
    );
    expect(authenticateOriginCookieMock).toHaveBeenCalledWith(user);
  });

  it('discards a pending direct bearer after cookie authentication succeeds', async () => {
    getCurrentUserViaConnectMock.mockResolvedValueOnce(user);
    const { loadCurrentUser, stagePendingOriginAuthentication } = await loadModule();
    stagePendingOriginAuthentication(pendingCredentials, 1000);

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledOnce();
    expect(authenticateOriginCookieMock).toHaveBeenCalledWith(user);
    expect(authenticateOriginBearerMock).not.toHaveBeenCalled();
  });

  it('persists a pending direct bearer only after the cookie probe is unauthenticated', async () => {
    originState.token = 'older-origin-token';
    getCurrentUserViaConnectMock
      .mockRejectedValueOnce({ message: 'authentication required' })
      .mockResolvedValueOnce(user);
    const { loadCurrentUser, stagePendingOriginAuthentication } = await loadModule();
    stagePendingOriginAuthentication(pendingCredentials, 1000);

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenNthCalledWith(1, {
      baseUrl: '/api/connect',
      bearerToken: null
    });
    expect(getCurrentUserViaConnectMock).toHaveBeenNthCalledWith(2, {
      baseUrl: '/api/connect',
      bearerToken: pendingCredentials.token
    });
    expect(authenticateOriginBearerMock).toHaveBeenCalledWith(pendingCredentials, user, 1000);
    expect(authenticateOriginCookieMock).not.toHaveBeenCalled();
  });

  it('clears a rejected pending direct bearer before a later cookie probe', async () => {
    getCurrentUserViaConnectMock
      .mockRejectedValueOnce({ message: 'authentication required' })
      .mockRejectedValueOnce({ message: 'authentication required' });
    const { loadCurrentUser, stagePendingOriginAuthentication } = await loadModule();
    stagePendingOriginAuthentication(pendingCredentials, 1000);

    expect(await loadCurrentUser()).toBeNull();

    getCurrentUserViaConnectMock.mockRejectedValueOnce({ message: 'authentication required' });
    expect(await loadCurrentUser()).toBeNull();
    expect(getCurrentUserViaConnectMock).toHaveBeenLastCalledWith({
      baseUrl: '/api/connect',
      bearerToken: null
    });
    expect(authenticateOriginBearerMock).not.toHaveBeenCalled();
  });

  it('uses a legacy origin bearer only when cookie authentication is absent', async () => {
    originState.token = 'legacy-origin-token';
    getCurrentUserViaConnectMock
      .mockRejectedValueOnce({ message: 'authentication required' })
      .mockResolvedValueOnce(user);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ bearerToken: null })
    );
    expect(getCurrentUserViaConnectMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ bearerToken: 'legacy-origin-token' })
    );
    expect(authenticateOriginCookieMock).not.toHaveBeenCalled();
  });

  it('adopts a renewed legacy bearer while retrying the cookie probe', async () => {
    originState.token = 'old-origin-token';
    getCurrentUserViaConnectMock
      .mockImplementationOnce(async () => {
        originState.token = 'renewed-origin-token';
        throw new Error('network');
      })
      .mockRejectedValueOnce({ message: 'authentication required' })
      .mockResolvedValueOnce(user);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenNthCalledWith(
      3,
      expect.objectContaining({ bearerToken: 'renewed-origin-token' })
    );
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

  it('keeps cached auth state for typed unavailable errors containing auth wording', async () => {
    const { loadCurrentUser } = await loadModule();
    const unavailable = new ConnectError(
      'authentication required: storage unavailable',
      Code.Unavailable
    );
    getCurrentUserViaConnectMock
      .mockResolvedValueOnce(user)
      .mockRejectedValueOnce(unavailable)
      .mockRejectedValueOnce(unavailable);

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual(user);
    expect(clearOriginAuthenticationMock).not.toHaveBeenCalled();
    expect(handleAuthenticationRequiredMock).not.toHaveBeenCalled();
  });
});
