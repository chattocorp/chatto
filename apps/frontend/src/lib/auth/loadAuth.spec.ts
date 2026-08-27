import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';

const {
  getCurrentUserViaConnectMock,
  clearOriginAuthenticationMock,
  handleAuthenticationRequiredMock,
  clearAuthenticationRequiredMock,
  authenticateOriginCookieMock,
  maintainBrowserSessionMock,
  revokeLegacyOriginBearerSessionMock,
  migrateLegacyOriginCookieSessionMock
} = vi.hoisted(() => ({
  getCurrentUserViaConnectMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn(),
  handleAuthenticationRequiredMock: vi.fn(),
  clearAuthenticationRequiredMock: vi.fn(),
  authenticateOriginCookieMock: vi.fn(),
  maintainBrowserSessionMock: vi.fn(),
  revokeLegacyOriginBearerSessionMock: vi.fn(),
  migrateLegacyOriginCookieSessionMock: vi.fn()
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
    originConnectBaseUrl: '/api/connect',
    originClient: { maintainBrowserSession: maintainBrowserSessionMock }
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    originServer: { id: 'origin', token: 'legacy-origin-token' },
    authenticateOriginCookie: authenticateOriginCookieMock,
    clearOriginAuthentication: clearOriginAuthenticationMock,
    handleAuthenticationRequired: handleAuthenticationRequiredMock,
    clearAuthenticationRequired: clearAuthenticationRequiredMock
  }
}));

vi.mock('./originBearerMigration', () => ({
  revokeLegacyOriginBearerSession: revokeLegacyOriginBearerSessionMock
}));

vi.mock('./legacyCookieMigration', () => ({
  migrateLegacyOriginCookieSession: migrateLegacyOriginCookieSessionMock
}));

const user = {
  id: 'U1',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  bio: null,
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
    revokeLegacyOriginBearerSessionMock.mockResolvedValue(undefined);
    migrateLegacyOriginCookieSessionMock.mockResolvedValue(false);
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
    expect(maintainBrowserSessionMock).toHaveBeenCalledTimes(2);
  });

  it('discards a legacy origin bearer after cookie authentication succeeds', async () => {
    getCurrentUserViaConnectMock.mockResolvedValueOnce(user);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledOnce();
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledWith(
      expect.objectContaining({ bearerToken: null })
    );
    expect(authenticateOriginCookieMock).toHaveBeenCalledWith(user);
    expect(revokeLegacyOriginBearerSessionMock).toHaveBeenCalledOnce();
  });

  it('keeps legacy bearer state when server-side revocation fails', async () => {
    getCurrentUserViaConnectMock.mockResolvedValueOnce(user).mockResolvedValueOnce(user);
    revokeLegacyOriginBearerSessionMock
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValueOnce(undefined);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(revokeLegacyOriginBearerSessionMock).toHaveBeenCalledTimes(2);
    expect(authenticateOriginCookieMock).toHaveBeenCalledOnce();
    expect(authenticateOriginCookieMock).toHaveBeenCalledWith(user);
  });

  it('revokes a stored origin bearer instead of using it after cookie authentication fails', async () => {
    getCurrentUserViaConnectMock.mockRejectedValueOnce({ message: 'authentication required' });
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toBeNull();
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledOnce();
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledWith({
      baseUrl: '/api/connect',
      bearerToken: null
    });
    expect(revokeLegacyOriginBearerSessionMock).toHaveBeenCalledOnce();
    expect(clearOriginAuthenticationMock).toHaveBeenCalledOnce();
  });

  it('migrates the previous browser cookie and retries authentication once', async () => {
    getCurrentUserViaConnectMock
      .mockRejectedValueOnce({ message: 'authentication required' })
      .mockResolvedValueOnce(user);
    migrateLegacyOriginCookieSessionMock.mockResolvedValueOnce(true);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(migrateLegacyOriginCookieSessionMock).toHaveBeenCalledOnce();
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledTimes(2);
    expect(authenticateOriginCookieMock).toHaveBeenCalledWith(user);
    expect(clearOriginAuthenticationMock).not.toHaveBeenCalled();
  });

  it('still verifies a migrated cookie after an earlier transient viewer error', async () => {
    getCurrentUserViaConnectMock
      .mockRejectedValueOnce(new Error('network'))
      .mockRejectedValueOnce({ message: 'authentication required' })
      .mockResolvedValueOnce(user);
    migrateLegacyOriginCookieSessionMock.mockResolvedValueOnce(true);
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toEqual(user);
    expect(getCurrentUserViaConnectMock).toHaveBeenCalledTimes(3);
    expect(migrateLegacyOriginCookieSessionMock).toHaveBeenCalledOnce();
    expect(authenticateOriginCookieMock).toHaveBeenCalledWith(user);
  });

  it('retains local authority when legacy cookie migration is temporarily unavailable', async () => {
    getCurrentUserViaConnectMock.mockRejectedValue({ message: 'authentication required' });
    migrateLegacyOriginCookieSessionMock.mockRejectedValue(new Error('unavailable'));
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toBeNull();
    expect(migrateLegacyOriginCookieSessionMock).toHaveBeenCalledTimes(2);
    expect(revokeLegacyOriginBearerSessionMock).not.toHaveBeenCalled();
    expect(clearOriginAuthenticationMock).not.toHaveBeenCalled();
  });

  it('keeps stored bearer authority when revocation after a failed cookie probe is unavailable', async () => {
    getCurrentUserViaConnectMock.mockRejectedValueOnce({ message: 'authentication required' });
    revokeLegacyOriginBearerSessionMock.mockRejectedValueOnce(new Error('unavailable'));
    const { loadCurrentUser } = await loadModule();

    expect(await loadCurrentUser()).toBeNull();
    expect(revokeLegacyOriginBearerSessionMock).toHaveBeenCalledOnce();
    expect(clearOriginAuthenticationMock).not.toHaveBeenCalled();
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
