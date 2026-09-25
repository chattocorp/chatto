import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    saveReturnUrl: vi.fn(),
    loadSavedView: vi.fn(),
    startServerNetwork: vi.fn(),
    serverId: 'origin' as string | null,
    origin: true,
    reauthRequiredAt: null as number | null,
    store: {
      restoreSavedView: vi.fn(),
      networkStartupDeferred: false,
      realtimeSync: { phase: 'empty' as 'empty' | 'stale' | 'ready' },
      savedView: null as { serverId: string; userId: string; rooms: unknown[] } | null,
      currentUser: {
        loading: false,
        user: { id: 'viewer-1' } as { id: string } | undefined,
        load: vi.fn()
      }
    }
  }
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$lib/auth/returnNavigation', () => ({
  saveReturnUrl: mocks.saveReturnUrl
}));

vi.mock('$lib/storage/savedViews', () => ({ loadSavedView: mocks.loadSavedView }));

vi.mock('$lib/navigation', () => ({
  segmentToServerId: () => mocks.serverId
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    startServerNetwork: mocks.startServerNetwork,
    tryGetStore: () => (mocks.serverId ? mocks.store : undefined),
    getServer: () =>
      mocks.serverId ? { id: mocks.serverId, reauthRequiredAt: mocks.reauthRequiredAt } : undefined,
    isOriginServer: () => mocks.origin
  }
}));

import { load } from './+layout';

function routeLoad(user: { id: string } | null = { id: 'viewer-1' }, setupRequired = false,
  startupServerId?: string) {
  return load({
    params: { serverId: '-' },
    parent: async () => ({ user, serverInfo: { setupRequired }, startupServerId }),
    url: new URL('https://chat.example.test/chat/-/overview')
  } as never);
}

async function expectLoginRedirect(user?: { id: string } | null): Promise<void> {
  await expect(routeLoad(user)).rejects.toMatchObject({ status: 302, location: '/login' });
  expect(mocks.saveReturnUrl).toHaveBeenCalledWith('/chat/-/overview');
}

describe('server route layout load', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.serverId = 'origin';
    mocks.origin = true;
    mocks.reauthRequiredAt = null;
    mocks.store.currentUser.loading = false;
    mocks.store.currentUser.user = { id: 'viewer-1' };
    mocks.store.currentUser.load.mockResolvedValue(undefined);
    mocks.store.savedView = null;
    mocks.store.networkStartupDeferred = false;
    mocks.store.realtimeSync.phase = 'empty';
    mocks.loadSavedView.mockResolvedValue(null);
    // The store accepts a matching view; tests override this to model refusal.
    mocks.store.restoreSavedView.mockImplementation((view: typeof mocks.store.savedView) => {
      if (view) mocks.store.savedView = view;
    });
  });

  it('opens setup for the origin before requiring authentication', async () => {
    await expect(routeLoad(null, true)).rejects.toMatchObject({ status: 302, location: '/setup' });
    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });

  it('allows an authenticated remote server while origin setup is pending', async () => {
    mocks.origin = false;
    mocks.serverId = 'remote';
    await expect(routeLoad(null, true)).resolves.toMatchObject({ serverSegment: '-' });
  });

  it('redirects an unresolved server before the layout component mounts', async () => {
    mocks.serverId = null;

    await expectLoginRedirect();
  });

  it('does not read the room param, so room switches do not re-run it', async () => {
    const params = new Proxy(
      { serverId: '-', roomId: 'room-1' },
      {
        get(target, key) {
          if (key === 'roomId') throw new Error('server layout load read params.roomId');
          return Reflect.get(target, key);
        }
      }
    );

    await expect(
      load({
        params,
        parent: async () => ({ user: { id: 'viewer-1' }, serverInfo: { setupRequired: false } }),
        url: new URL('https://chat.example.test/chat/-/room-1')
      } as never)
    ).resolves.toEqual({ serverSegment: '-' });
  });

  it('uses the parent origin viewer without a second viewer request', async () => {
    await expect(routeLoad()).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.store.currentUser.load).not.toHaveBeenCalled();
  });

  it('redirects an unauthenticated origin and records its return URL', async () => {
    await expectLoginRedirect(null);
  });

  it('waits for the initial remote viewer request before deciding access', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    mocks.store.currentUser.load.mockImplementation(async () => {
      mocks.store.currentUser.loading = false;
      mocks.store.currentUser.user = { id: 'remote-viewer' };
    });

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.store.currentUser.load).toHaveBeenCalledOnce();
    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });

  it('opens a saved remote view without waiting for its viewer request', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    const savedView = { serverId: 'remote', userId: 'remote-viewer', rooms: [] };
    mocks.loadSavedView.mockResolvedValue(savedView);

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });
    expect(mocks.store.restoreSavedView).toHaveBeenCalledWith(savedView, false);
    expect(mocks.store.currentUser.load).not.toHaveBeenCalled();
  });

  it('does not treat a refused saved view as a readable view', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    mocks.loadSavedView.mockResolvedValue({ serverId: 'remote', userId: 'viewer-1', rooms: [] });
    mocks.store.restoreSavedView.mockImplementation(() => {});

    await expect(routeLoad(null)).rejects.toMatchObject({ status: 302, location: '/login' });
    expect(mocks.store.currentUser.load).toHaveBeenCalledOnce();
  });

  it('restores a dormant server before starting its requests', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.networkStartupDeferred = true;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    const savedView = { serverId: 'remote', userId: 'viewer-1', rooms: [] };
    mocks.loadSavedView.mockResolvedValue(savedView);

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.store.restoreSavedView).toHaveBeenCalledWith(savedView, true);
    expect(mocks.store.restoreSavedView.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.startServerNetwork.mock.invocationCallOrder[0]
    );
  });

  it('leaves network startup to the root layout after the first saved paint', async () => {
    mocks.store.savedView = { serverId: 'origin', userId: 'viewer-1', rooms: [] };
    mocks.store.realtimeSync.phase = 'stale';

    await expect(routeLoad(null, false, 'origin')).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.startServerNetwork).not.toHaveBeenCalled();
    expect(mocks.loadSavedView).not.toHaveBeenCalled();
  });

  it('does not read saved views for a store that already has a projection', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.realtimeSync.phase = 'ready';

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.loadSavedView).not.toHaveBeenCalled();
    expect(mocks.store.restoreSavedView).not.toHaveBeenCalled();
    expect(mocks.startServerNetwork).toHaveBeenCalledWith('remote');
  });

  it('keeps the shell mounted for reauthentication recovery', async () => {
    mocks.reauthRequiredAt = Date.now();
    mocks.store.currentUser.user = undefined;

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });

  it('keeps the shell mounted when the initial remote viewer request requires reauthentication', async () => {
    mocks.serverId = 'remote';
    mocks.origin = false;
    mocks.store.currentUser.loading = true;
    mocks.store.currentUser.user = undefined;
    mocks.store.currentUser.load.mockImplementation(async () => {
      mocks.store.currentUser.loading = false;
      mocks.reauthRequiredAt = Date.now();
    });

    await expect(routeLoad(null)).resolves.toMatchObject({ serverSegment: '-' });

    expect(mocks.saveReturnUrl).not.toHaveBeenCalled();
  });
});
