import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { RealtimeResourceUpdate } from '$lib/api-client/realtimeResources';

const mocks = vi.hoisted(() => ({
  viewer: vi.fn()
}));

vi.mock('$app/state', () => ({
  page: { route: { id: '/chat/[serverId]/[roomId]' }, params: { serverId: '-' } }
}));
vi.mock('$lib/auth/legacyCookieMigration', () => ({
  migrateLegacyOriginCookieSession: async () => false
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
    store.currentUser.loading = false;
    store.projection.rooms.set(
      'R1',
      new RoomWithViewerState({
        room: { id: 'R1', name: 'general' },
        viewerState: { isMember: true }
      })
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

      serverRegistry.authenticateOriginCookie({
        id: 'U1',
        login: 'one',
        displayName: 'One',
        presenceStatus: PresenceStatus.ONLINE,
        hasVerifiedEmail: false,
        hasPassword: true,
        viewerCanDeleteAccount: true
      });

      const origin = serverRegistry.originServer!;
      expect(serverRegistry.getStore(origin.id).currentUser.verifiedUserId).toBe('U1');
      expect(renew).toHaveBeenCalledOnce();
    }
  );

  it('verifies the same viewer without replacing the retained store', async () => {
    const store = retainedStore();
    const renew = vi
      .spyOn(ServerConnection.prototype, 'maintainBrowserSession')
      .mockImplementation(() => {});
    const rooms = store.navigation.rooms;
    mocks.viewer.mockResolvedValueOnce({ id: 'U1', login: 'one', displayName: 'One' });

    await serverRegistry.recoverServer('origin');

    expect(mocks.viewer).toHaveBeenCalledOnce();
    expect(serverRegistry.getStore('origin')).toBe(store);
    expect(store.navigation.rooms).toEqual(rooms);
    expect(store.isAuthenticated).toBe(true);
    expect(renew).toHaveBeenCalledOnce();
  });

  it('shares the account request between route loading and recovery', async () => {
    const store = retainedStore();
    const { loadCurrentUser } = await import('$lib/auth/loadAuth');
    let finish!: (user: { id: string; login: string; displayName: string }) => void;
    mocks.viewer.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    vi.spyOn(ServerConnection.prototype, 'maintainBrowserSession').mockImplementation(() => {});
    const route = loadCurrentUser();
    const recovery = serverRegistry.recoverServer('origin');
    expect(mocks.viewer).toHaveBeenCalledOnce();
    expect(store.currentUser.user).toBeUndefined();
    finish({ id: 'U1', login: 'one', displayName: 'One' });
    const [user] = await Promise.all([route, recovery]);
    expect(user).toBe(store.currentUser.user);
    expect(store.isAuthenticated).toBe(true);
    expect(mocks.viewer).toHaveBeenCalledOnce();
  });

  it('does not publish a response after the server account is cleared', async () => {
    const store = retainedStore();
    let finish!: (user: { id: string; login: string; displayName: string }) => void;
    mocks.viewer.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const pending = serverRegistry.recoverServer('origin');
    await vi.waitFor(() => expect(mocks.viewer).toHaveBeenCalledOnce());
    serverRegistry.clearServerAuthentication('origin');
    finish({ id: 'U1', login: 'one', displayName: 'One' });
    await pending;
    expect(store.currentUser.user).toBeUndefined();
    expect(serverRegistry.getStore('origin').currentUser.user).toBeUndefined();
    expect(serverRegistry.getStore('origin').isAuthenticated).toBe(false);
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

  it('does not let an older successful account response undo authentication loss', async () => {
    const store = retainedStore();
    let finish!: (user: { id: string; login: string; displayName: string }) => void;
    mocks.viewer.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const pending = serverRegistry.recoverServer('origin');
    await vi.waitFor(() => expect(mocks.viewer).toHaveBeenCalledOnce());
    serverRegistry.handleAuthenticationRequired('origin');
    finish({ id: 'U1', login: 'one', displayName: 'One' });
    await pending;
    expect(store.currentUser.user).toBeUndefined();
    expect(store.isAuthenticated).toBe(false);
    expect(serverRegistry.getServer('origin')?.reauthRequiredAt).not.toBeNull();
  });

  it('applies the same account boundary to live viewer resources', () => {
    const previous = retainedStore();
    vi.spyOn(ServerConnection.prototype, 'maintainBrowserSession').mockImplementation(() => {});
    previous.realtimeProjectionHandler(
      new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'viewer',
            value: new GetViewerResponse({
              user: { profile: { id: 'U2', login: 'two', displayName: 'Two' } }
            })
          }
        })
      })
    );
    const current = serverRegistry.getStore('origin');
    expect(current).not.toBe(previous);
    expect(previous.currentUser.user).toBeUndefined();
    expect(current.currentUser.user?.id).toBe('U2');
    expect(current.projection.rooms.has('R1')).toBe(false);
  });

  it('keeps the reauthentication notice while loaded data remains readable', () => {
    const store = retainedStore();
    store.realtimeSync.markCaughtUp('live');

    serverRegistry.handleAuthenticationRequired('origin');

    expect(serverRegistry.getServer('origin')?.reauthRequiredAt).not.toBeNull();
    expect(serverRegistry.originSignInRequired).toBe(false);
  });

  it('does not require sign-in while the origin session is valid', () => {
    retainedStore();
    expect(serverRegistry.originSignInRequired).toBe(false);
  });
});
