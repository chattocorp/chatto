import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    servers: [] as Array<{ id: string; url: string; token: string | null }>,
    originId: 'origin',
    beginExplicitSignOutRedirect: vi.fn(),
    cancelExplicitSignOutRedirect: vi.fn(),
    signOutServer: vi.fn(),
    unsubscribePushBeforeLeaving: vi.fn(),
    notifyLogout: vi.fn(),
    clearLastRoom: vi.fn(),
    clearServerAuthentication: vi.fn(),
    removeServer: vi.fn()
  }
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    getServer: (id: string) => mocks.servers.find((server) => server.id === id),
    isOriginServer: (id: string) => id === mocks.originId,
    clearServerAuthentication: mocks.clearServerAuthentication,
    removeServer: mocks.removeServer
  }
}));

vi.mock('@chatto/client/auth/signOut', () => ({
  beginExplicitSignOutRedirect: mocks.beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect: mocks.cancelExplicitSignOutRedirect,
  ServerLogoutRejectedError: class ServerLogoutRejectedError extends Error {},
  signOutServer: mocks.signOutServer
}));
vi.mock('$lib/auth/sessionChannel', () => ({ notifyLogout: mocks.notifyLogout }));
vi.mock('$lib/notifications/pushNotifications', () => ({
  unsubscribeBeforeLeaving: mocks.unsubscribePushBeforeLeaving
}));
vi.mock('$lib/storage/lastRoom', () => ({ clearLastRoom: mocks.clearLastRoom }));

import { clientAccount } from './clientAccount';

beforeEach(() => {
  vi.clearAllMocks();
  mocks.servers = [
    { id: 'origin', url: 'https://origin.example', token: 'origin-token' },
    { id: 'remote', url: 'https://remote.example', token: 'remote-token' }
  ];
  mocks.originId = 'origin';
  mocks.signOutServer.mockResolvedValue(new Response('{}'));
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
    expect(result).toEqual({ kind: 'soft', serverId: 'remote' });
  });

  it('keeps the origin registration while clearing its local session', async () => {
    const result = await clientAccount.signOutCurrentServer('origin');

    expect(mocks.beginExplicitSignOutRedirect).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledWith('origin');
    expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin');
    expect(mocks.removeServer).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).toHaveBeenCalledOnce();
    expect(result).toEqual({ kind: 'hard', serverId: 'origin' });
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
    const { ServerLogoutRejectedError } = await import('@chatto/client/auth/signOut');
    mocks.signOutServer.mockRejectedValueOnce(new ServerLogoutRejectedError(503));

    await expect(clientAccount.signOutCurrentServer('origin')).rejects.toBeInstanceOf(
      ServerLogoutRejectedError
    );

    expect(mocks.cancelExplicitSignOutRedirect).toHaveBeenCalledOnce();
    expect(mocks.clearLastRoom).not.toHaveBeenCalled();
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
  });
});
