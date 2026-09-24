import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  viewer: vi.fn()
}));

vi.mock('$app/state', () => ({
  page: { route: { id: '/chat/[serverId]/[roomId]' }, params: { serverId: '-' } }
}));

vi.mock('$lib/api-client/viewer', async (original) => ({
  ...(await original<typeof import('$lib/api-client/viewer')>()),
  getCurrentUserViaConnect: mocks.viewer
}));

vi.mock('$lib/api-client/server', () => ({
  getPublicServerInfo: vi.fn(async () => ({
    name: 'Chatto',
    version: '0.5.0',
    welcomeMessage: null,
    description: null,
    iconUrl: null,
    bannerUrl: null,
    directRegistrationEnabled: true,
    directLoginEnabled: true
  }))
}));

import { serverRegistry } from './registry.svelte';
import { ServerConnection } from './serverConnection.svelte';
import { emptyServerSession } from './sessions.svelte';
import ServerRuntimeCoordinator from './ServerRuntimeCoordinator.svelte';

describe('origin startup recovery', () => {
  beforeEach(() => {
    mocks.viewer.mockReset();
  });

  afterEach(() => {
    serverRegistry.removeAll();
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  function retainedStore() {
    serverRegistry.removeAll();
    serverRegistry.addServer(
      {
        id: 'origin',
        url: window.location.origin,
        name: 'Chatto',
        iconUrl: null,
        addedAt: Date.now()
      },
      { ...emptyServerSession(), userId: 'U1' }
    );
    const store = serverRegistry.getStore('origin');
    store.restoreSavedView(
      {
        version: 1,
        serverId: 'origin',
        userId: 'U1',
        serverName: 'Chatto',
        savedAt: Date.now(),
        rooms: [{ id: 'R1', name: 'general', messages: [] }]
      },
      true
    );
    return store;
  }

  it.each([null, 'U1', 'previous-viewer'])(
    'verifies normal cookie authentication with previous viewer %s',
    (previousUserId) => {
      serverRegistry.removeAll();
      const renew = vi
        .spyOn(ServerConnection.prototype, 'maintainBrowserSession')
        .mockImplementation(() => {});
      if (previousUserId) {
        serverRegistry.addServer(
          { id: 'origin', url: window.location.origin, name: 'Chatto', iconUrl: null, addedAt: 1 },
          { ...emptyServerSession(), userId: previousUserId }
        );
        expect(serverRegistry.getStore('origin').currentUser.verifiedUserId).toBeNull();
      }

      serverRegistry.authenticateOriginCookie({ id: 'U1', login: 'one' });

      const origin = serverRegistry.originServer!;
      expect(serverRegistry.getStore(origin.id).currentUser.verifiedUserId).toBe('U1');
      expect(renew).toHaveBeenCalledOnce();
    }
  );

  it.each(['timer', 'online'] as const)(
    'retries a failed saved viewer through %s while realtime stays deferred',
    async (trigger) => {
      const store = retainedStore();
      vi.spyOn(ServerConnection.prototype, 'maintainBrowserSession').mockImplementation(() => {});
      vi.spyOn(console, 'error').mockImplementation(() => {});
      mocks.viewer
        .mockRejectedValueOnce(new Error('offline'))
        .mockResolvedValueOnce({ id: 'U1', login: 'one', displayName: 'One' });
      const view = render(ServerRuntimeCoordinator, {
        props: { user: null, deferConnections: true }
      });
      try {
        await serverRegistry.recoverServer('origin');
        expect(store.startupPresentationOnly).toBe(true);
        expect(store.isAuthenticated).toBe(false);
        expect(mocks.viewer).toHaveBeenCalledOnce();

        if (trigger === 'online') window.dispatchEvent(new Event('online'));

        await vi.waitFor(() => expect(store.isAuthenticated).toBe(true), { timeout: 4_000 });
        expect(mocks.viewer).toHaveBeenCalledTimes(2);
        expect(store.currentUser.verifiedUserId).toBe('U1');
        expect(store.projection.rooms.has('R1')).toBe(true);
      } finally {
        view.unmount();
      }
    }
  );

  it('keeps a disk view read-only after a transient viewer failure', async () => {
    const store = retainedStore();
    mocks.viewer.mockRejectedValueOnce(new Error('offline'));
    vi.spyOn(console, 'error').mockImplementation(() => {});

    await serverRegistry.recoverServer('origin');

    expect(store.startupPresentationOnly).toBe(true);
    expect(store.isAuthenticated).toBe(false);
    expect(store.projection.rooms.has('R1')).toBe(true);
    vi.restoreAllMocks();
  });

  it('verifies the same viewer without replacing the retained store', async () => {
    const store = retainedStore();
    const renew = vi
      .spyOn(ServerConnection.prototype, 'maintainBrowserSession')
      .mockImplementation(() => {});
    const room = store.projection.rooms.get('R1');
    mocks.viewer.mockResolvedValueOnce({ id: 'U1', login: 'one', displayName: 'One' });

    await serverRegistry.recoverServer('origin');

    expect(mocks.viewer).toHaveBeenCalledOnce();
    expect(serverRegistry.getStore('origin')).toBe(store);
    expect(store.projection.rooms.get('R1')).toBe(room);
    expect(store.startupPresentationOnly).toBe(false);
    expect(store.isAuthenticated).toBe(true);
    expect(renew).toHaveBeenCalledOnce();
  });

  it('discards the previous account view when the verified viewer differs', async () => {
    const store = retainedStore();
    const renew = vi
      .spyOn(ServerConnection.prototype, 'maintainBrowserSession')
      .mockImplementation(() => {});
    mocks.viewer.mockResolvedValueOnce({ id: 'U2', login: 'two', displayName: 'Two' });

    await serverRegistry.recoverServer('origin');

    expect(serverRegistry.getStore('origin')).not.toBe(store);
    expect(serverRegistry.getStore('origin').projection.rooms.has('R1')).toBe(false);
    expect(serverRegistry.getServer('origin')?.userId).toBe('U2');
    expect(serverRegistry.getStore('origin').currentUser.verifiedUserId).toBe('U2');
    expect(serverRegistry.getStore('origin').isAuthenticated).toBe(true);
    expect(renew).toHaveBeenCalledOnce();
  });

  it('clears the saved private view after the viewer is rejected', async () => {
    const store = retainedStore();
    mocks.viewer.mockRejectedValueOnce(new ConnectError('unauthenticated', Code.Unauthenticated));

    await serverRegistry.recoverServer('origin');

    expect(store.projection.rooms.has('R1')).toBe(false);
    expect(store.startupPresentationOnly).toBe(false);
    expect(store.isAuthenticated).toBe(false);
  });
});
