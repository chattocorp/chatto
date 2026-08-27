import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    servers: [] as Array<{ id: string; url: string; token: string | null }>,
    originId: 'origin',
    authenticated: new Set<string>(),
    beginExplicitSignOutRedirect: vi.fn(),
    cancelExplicitSignOutRedirect: vi.fn(),
    signOutServer: vi.fn(),
    signOutServers: vi.fn(),
    unsubscribePushBeforeLeaving: vi.fn(),
    notifyLogout: vi.fn(),
    clearLastRoom: vi.fn(),
    clearServerAuthentication: vi.fn(),
    removeServer: vi.fn(),
    resetToOrigin: vi.fn()
  }
}));

vi.mock('$lib/auth/signOut', () => ({
  beginExplicitSignOutRedirect: mocks.beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect: mocks.cancelExplicitSignOutRedirect,
  ServerLogoutRejectedError: class ServerLogoutRejectedError extends Error {},
  signOutServer: mocks.signOutServer,
  signOutServers: mocks.signOutServers
}));
vi.mock('$lib/auth/sessionChannel', () => ({ notifyLogout: mocks.notifyLogout }));
vi.mock('$lib/notifications/pushNotifications', () => ({
  unsubscribeBeforeLeaving: mocks.unsubscribePushBeforeLeaving
}));
vi.mock('$lib/storage/lastRoom', () => ({ clearLastRoom: mocks.clearLastRoom }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    getServer: (id: string) => mocks.servers.find((server) => server.id === id),
    isOriginServer: (id: string) => id === mocks.originId,
    firstAuthenticatedServerId: (excludedId?: string) =>
      mocks.servers.find((server) => server.id !== excludedId && mocks.authenticated.has(server.id))
        ?.id,
    clearServerAuthentication: mocks.clearServerAuthentication,
    removeServer: mocks.removeServer,
    resetToOrigin: mocks.resetToOrigin
  }
}));

import { clientAccount } from './clientAccount';

beforeEach(() => {
  vi.clearAllMocks();
  mocks.servers = [
    { id: 'origin', url: 'https://origin.example', token: 'origin-token' },
    { id: 'remote', url: 'https://remote.example', token: 'remote-token' }
  ];
  mocks.originId = 'origin';
  mocks.authenticated = new Set(['origin', 'remote']);
  mocks.signOutServer.mockResolvedValue(new Response('{}'));
  mocks.signOutServers.mockResolvedValue(undefined);
  mocks.unsubscribePushBeforeLeaving.mockResolvedValue(undefined);
});

describe('ClientAccountCoordinator', () => {
  it('keeps a remote registration while clearing its local session', async () => {
    const result = await clientAccount.signOutCurrentServer('remote');

    expect(mocks.signOutServer).toHaveBeenCalledWith(mocks.servers[1], false);
    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledWith('remote');
    expect(mocks.clearLastRoom).toHaveBeenCalledWith('remote');
    expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('remote');
    expect(mocks.removeServer).not.toHaveBeenCalled();
    expect(result).toEqual({ kind: 'soft', serverId: 'origin' });
  });

  it('keeps the origin registration while clearing its local session', async () => {
    const result = await clientAccount.signOutCurrentServer('origin');

    expect(mocks.beginExplicitSignOutRedirect).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledWith('origin');
    expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin');
    expect(mocks.removeServer).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).toHaveBeenCalledOnce();
    expect(result).toEqual({ kind: 'hard', serverId: 'remote' });
  });

  it('clears local state after signing out every server', async () => {
    const result = await clientAccount.signOutAllServers();

    expect(mocks.signOutServers).toHaveBeenCalledWith(mocks.servers, expect.any(Function));
    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledTimes(2);
    expect(mocks.resetToOrigin).toHaveBeenCalledOnce();
    expect(mocks.notifyLogout).toHaveBeenCalledOnce();
    expect(result).toEqual({ kind: 'hard' });
  });

  it('preserves authentication when push cleanup cannot establish a delivery fence', async () => {
    mocks.unsubscribePushBeforeLeaving.mockRejectedValueOnce(new Error('browser lookup failed'));

    await expect(clientAccount.signOutCurrentServer('origin')).rejects.toThrow(
      'browser lookup failed'
    );

    expect(mocks.beginExplicitSignOutRedirect).not.toHaveBeenCalled();
    expect(mocks.signOutServer).not.toHaveBeenCalled();
    expect(mocks.clearLastRoom).not.toHaveBeenCalled();
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
  });

  it('preserves authentication when the server rejects logout', async () => {
    const { ServerLogoutRejectedError } = await import('$lib/auth/signOut');
    mocks.signOutServer.mockRejectedValueOnce(new ServerLogoutRejectedError(503));

    await expect(clientAccount.signOutCurrentServer('origin')).rejects.toBeInstanceOf(
      ServerLogoutRejectedError
    );

    expect(mocks.cancelExplicitSignOutRedirect).toHaveBeenCalledOnce();
    expect(mocks.clearLastRoom).not.toHaveBeenCalled();
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
  });

  it('preserves all-server authentication when any push cleanup cannot establish a delivery fence', async () => {
    mocks.unsubscribePushBeforeLeaving
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('remote browser lookup failed'));

    await expect(clientAccount.signOutAllServers()).rejects.toThrow('remote browser lookup failed');

    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledTimes(2);
    expect(mocks.beginExplicitSignOutRedirect).not.toHaveBeenCalled();
    expect(mocks.signOutServers).not.toHaveBeenCalled();
    expect(mocks.resetToOrigin).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
  });
});
