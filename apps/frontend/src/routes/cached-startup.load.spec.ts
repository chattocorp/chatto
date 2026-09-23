import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  preloadPublicLocaleMessages: vi.fn(),
  getPublicServerInfo: vi.fn(),
  loadCurrentUser: vi.fn(),
  loadSavedView: vi.fn(),
  getLastRoom: vi.fn(),
  explicitSignOut: vi.fn(),
  restoreSavedView: vi.fn(),
  init: vi.fn(),
  startServerNetwork: vi.fn(),
  probeOrigin: vi.fn(),
  settleOriginUnauthenticated: vi.fn(),
  userId: 'U1' as string | null,
  startupPresentationOnly: true
}));

vi.mock('$lib/i18n/messages', () => ({
  preloadPublicLocaleMessages: mocks.preloadPublicLocaleMessages
}));
vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo: mocks.getPublicServerInfo }));
vi.mock('$lib/auth/loadAuth', () => ({ loadCurrentUser: mocks.loadCurrentUser }));
vi.mock('$lib/auth/signOut', () => ({
  isExplicitSignOutRedirectInProgress: mocks.explicitSignOut
}));
vi.mock('$lib/navigation', () => ({ segmentToServerId: () => 'origin' }));
vi.mock('$lib/storage/savedViews', () => ({ loadSavedView: mocks.loadSavedView }));
vi.mock('$lib/storage/lastRoom', () => ({ getLastRoom: mocks.getLastRoom }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    init: mocks.init,
    startServerNetwork: mocks.startServerNetwork,
    servers: [{ id: 'origin' }],
    getServer: () => ({ userId: mocks.userId }),
    getStore: () => ({ restoreSavedView: mocks.restoreSavedView }),
    tryGetStore: () => ({ startupPresentationOnly: mocks.startupPresentationOnly }),
    originServer: { id: 'origin' },
    probeOrigin: mocks.probeOrigin,
    settleOriginUnauthenticated: mocks.settleOriginUnauthenticated
  }
}));

function route(path = '/chat/-/R1') {
  return {
    url: new URL(`https://chat.example.test${path}`),
    params: { serverId: '-' }
  } as never;
}

describe('saved startup route load', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.clearAllMocks();
    mocks.userId = 'U1';
    mocks.startupPresentationOnly = true;
    mocks.explicitSignOut.mockReturnValue(false);
    mocks.preloadPublicLocaleMessages.mockResolvedValue(undefined);
    mocks.getPublicServerInfo.mockResolvedValue({ name: 'Live server' });
    mocks.loadCurrentUser.mockResolvedValue({ id: 'U1' });
    mocks.probeOrigin.mockResolvedValue(undefined);
    mocks.loadSavedView.mockResolvedValue({
      version: 1, serverId: 'origin', userId: 'U1', serverName: 'Saved server',
      savedAt: Date.now(), rooms: [{ id: 'R1', name: 'general', messages: [] }]
    });
    mocks.getLastRoom.mockReturnValue(null);
  });

  it('keeps the cold saved-view path through the installed app landing redirect', async () => {
    mocks.getLastRoom.mockReturnValue('R1');
    const { load } = await import('./+layout');

    await expect(load(route('/chat/-'))).rejects.toMatchObject({
      status: 302, location: '/chat/-/R1'
    });
    expect(mocks.loadCurrentUser).not.toHaveBeenCalled();

    await expect(load(route())).resolves.toMatchObject({ startupPending: true });
    expect(mocks.loadCurrentUser).not.toHaveBeenCalled();
  });

  it('restores the normal view before starting discovery or viewer requests', async () => {
    const { load } = await import('./+layout');
    const first = await load(route());

    expect(first).toMatchObject({ startupPending: true, startupServerId: 'origin', user: null });
    expect(first).not.toHaveProperty('startupSavedView');
    expect(mocks.init).toHaveBeenCalledWith(true);
    expect(mocks.restoreSavedView).toHaveBeenCalledWith(
      expect.objectContaining({ userId: 'U1' }), true
    );
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
    expect(mocks.loadCurrentUser).not.toHaveBeenCalled();

    const second = await load(route());
    expect(second).toMatchObject({ user: { id: 'U1' }, serverInfo: { name: 'Live server' } });
    expect(mocks.init).toHaveBeenLastCalledWith(true);
    expect(mocks.startServerNetwork).toHaveBeenCalledWith('origin');
    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
    expect(mocks.loadCurrentUser).toHaveBeenCalledOnce();
  });

  it('uses live startup for a message permalink outside the saved window', async () => {
    const { load } = await import('./+layout');
    const result = await load({
      url: new URL('https://chat.example.test/chat/-/R1/m/E1'),
      params: { serverId: '-', roomId: 'R1', messageId: 'E1' }
    } as never);

    expect(result).not.toHaveProperty('startupPending');
    expect(mocks.loadSavedView).not.toHaveBeenCalled();
    expect(mocks.restoreSavedView).not.toHaveBeenCalled();
    expect(mocks.getPublicServerInfo).toHaveBeenCalledOnce();
    expect(mocks.loadCurrentUser).toHaveBeenCalledOnce();
  });

  it('does not restore a view after its local account changes during the disk read', async () => {
    let finishRead: (view: unknown) => void = () => {};
    mocks.loadSavedView.mockReturnValue(new Promise((resolve) => { finishRead = resolve; }));
    const { load } = await import('./+layout');
    const pending = load(route());
    mocks.userId = null;
    finishRead({ version: 1, serverId: 'origin', userId: 'U1', rooms: [] });

    const result = await pending;
    expect(result).not.toHaveProperty('startupPending');
    expect(mocks.restoreSavedView).not.toHaveBeenCalled();
    expect(mocks.loadCurrentUser).toHaveBeenCalledOnce();
  });

  it('does not restore a view when sign-out starts during the disk read', async () => {
    let finishRead: (view: unknown) => void = () => {};
    mocks.loadSavedView.mockReturnValue(new Promise((resolve) => { finishRead = resolve; }));
    const { load } = await import('./+layout');
    const pending = load(route());
    mocks.explicitSignOut.mockReturnValue(true);
    finishRead({ version: 1, serverId: 'origin', userId: 'U1', rooms: [] });

    expect(await pending).not.toHaveProperty('startupPending');
    expect(mocks.restoreSavedView).not.toHaveBeenCalled();
  });
});
