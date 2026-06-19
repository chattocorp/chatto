import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { ServerStateStore } from './store.svelte';
import { eventBusManager } from './eventBus.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { RegisteredServer } from './registry.svelte';

class FakeServerConnection {
  reconnectCount = $state(0);
  wireUrl = 'ws://example.test/api/wire';
  token = 'test-token';
  results: unknown[];

  constructor(results: unknown[]) {
    this.results = results;
  }
}

const registered: RegisteredServer = {
  id: 'store-event-test',
  url: 'https://store-event.test',
  name: 'Store Event Test',
  iconUrl: null,
  token: 'remote-token',
  userId: 'U1',
  userLogin: 'alice',
  userDisplayName: 'Alice',
  userAvatarUrl: null,
  addedAt: 1
};

const cookieRegistered: RegisteredServer = {
  ...registered,
  token: null
};

function deferred<T = void>(): {
  promise: Promise<T>;
  resolve: (value: T | PromiseLike<T>) => void;
  reject: (reason?: unknown) => void;
} {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const stores: ServerStateStore[] = [];

function makeStore(fake: FakeServerConnection, server: RegisteredServer = registered): ServerStateStore {
  const store = new ServerStateStore(server, fake as unknown as ServerConnection);
  stores.push(store);
  return store;
}

async function flushPromises(times = 5): Promise<void> {
  for (let i = 0; i < times; i++) {
    await Promise.resolve();
  }
}

afterEach(() => {
  for (const store of stores.splice(0)) {
    store.dispose();
  }
  eventBusManager.stopBus(registered.id);
  vi.restoreAllMocks();
});

describe('ServerStateStore live server updates', () => {
  it('refreshes public profile and authenticated settings on ServerUpdatedEvent', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.serverInfo.refreshProfile = vi.fn().mockImplementation(async () => {
      store.serverInfo.name = 'Fresh Name';
      store.serverInfo.welcomeMessage = 'Fresh welcome';
      store.serverInfo.description = 'Fresh description';
      store.serverInfo.iconUrl = 'https://cdn/icon.webp';
      store.serverInfo.bannerUrl = 'https://cdn/banner.webp';
    });
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockImplementation(async () => {
      store.serverInfo.motd = 'Fresh MOTD';
      store.serverInfo.pushNotificationsEnabled = true;
      store.serverInfo.livekitUrl = 'wss://livekit';
      store.serverInfo.videoProcessingEnabled = true;
      store.serverInfo.maxUploadSize = 100;
      store.serverInfo.maxVideoUploadSize = 200;
      store.serverInfo.messageEditWindowSeconds = 120;
    });
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await flushPromises();
    await Promise.resolve();
    vi.mocked(store.serverInfo.refreshProfile).mockClear();
    vi.mocked(store.serverInfo.refreshAuthenticatedSettings).mockClear();

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E1',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: {
          __typename: 'ServerUpdatedEvent',
          name: 'stale',
          description: null,
          logoUrl: null,
          bannerUrl: null
        }
      });
    }
    await Promise.resolve();
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledOnce();
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledOnce();
    expect(store.serverInfo.name).toBe('Fresh Name');
    expect(store.serverInfo.welcomeMessage).toBe('Fresh welcome');
    expect(store.serverInfo.description).toBe('Fresh description');
    expect(store.serverInfo.iconUrl).toBe('https://cdn/icon.webp');
    expect(store.serverInfo.bannerUrl).toBe('https://cdn/banner.webp');
    expect(store.serverInfo.motd).toBe('Fresh MOTD');
    expect(store.serverInfo.pushNotificationsEnabled).toBe(true);
    expect(store.serverInfo.livekitUrl).toBe('wss://livekit');
  });

  it('forwards RoomGroupsUpdatedEvent once to every room-state store', async () => {
    const fake = new FakeServerConnection([
      {
        server: {
          pushNotificationsEnabled: false,
          vapidPublicKey: null,
          livekitUrl: null,
          videoProcessingEnabled: false,
          maxUploadSize: 25,
          maxVideoUploadSize: 25,
          messageEditWindowSeconds: 3600,
          profile: { motd: null }
        }
      },
      { server: { rooms: [] } },
      { server: { rooms: [] } },
      { server: { rooms: [], roomGroups: [] } },
      {
        server: {
          rooms: [{ id: 'r1', name: 'general', description: null, archived: false }],
          roomGroups: [{ id: 'g1', name: 'Lobby', rooms: [{ id: 'r1' }] }]
        }
      },
      { server: { rooms: [] } },
      { server: { rooms: [] } }
    ]);
    const store = makeStore(fake);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await Promise.resolve();
    await Promise.resolve();
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E2',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: { __typename: 'RoomGroupsUpdatedEvent', changed: true }
      });
    }
    await Promise.resolve();
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes projected server state for bearer-auth sessions', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.serverInfo.livekitUrl = 'wss://livekit';
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);
    store.activeCallRooms.load = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledOnce();
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledOnce();
    expect(store.notifications.fetch).toHaveBeenCalledOnce();
    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
    expect(store.activeCallRooms.load).toHaveBeenCalledOnce();
  });

  it('refreshes projected server state for cookie-auth sessions', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake, cookieRegistered);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await flushPromises();
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledOnce();
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledOnce();
    expect(store.notifications.fetch).toHaveBeenCalledOnce();
    expect(store.rooms.refresh).toHaveBeenCalledOnce();
    expect(store.roomDirectory.refresh).toHaveBeenCalledOnce();
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledOnce();
  });

  it('runs one queued projected-state refresh after an in-flight catch-up succeeds', async () => {
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const rooms = deferred();
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockReturnValueOnce(rooms.promise).mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('subscription-ended');
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();

    rooms.resolve();
    await vi.waitFor(() => expect(store.rooms.refresh).toHaveBeenCalledTimes(2));

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledTimes(2);
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledTimes(2);
    expect(store.notifications.fetch).toHaveBeenCalledTimes(2);
    expect(store.roomDirectory.refresh).toHaveBeenCalledTimes(2);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);
  });

  it('runs a queued projected-state refresh after the in-flight catch-up fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    const rooms = deferred();
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi.fn().mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockReturnValueOnce(rooms.promise).mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('subscription-ended');
      handler('ws-reconnected');
    }
    await Promise.resolve();

    expect(store.rooms.refresh).toHaveBeenCalledOnce();

    rooms.reject(new Error('network waking'));
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(store.serverInfo.refreshProfile).toHaveBeenCalledTimes(2);
    expect(store.serverInfo.refreshAuthenticatedSettings).toHaveBeenCalledTimes(2);
    expect(store.notifications.fetch).toHaveBeenCalledTimes(2);
    expect(store.rooms.refresh).toHaveBeenCalledTimes(2);
    expect(store.roomDirectory.refresh).toHaveBeenCalledTimes(2);
    expect(store.adminRoomLayout.refresh).toHaveBeenCalledTimes(2);
    expect(consoleError).toHaveBeenCalledOnce();
  });

  it('does not dedupe the next projected-state catch-up after a failed refresh', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fake = new FakeServerConnection([]);
    const store = makeStore(fake);
    store.serverInfo.refreshProfile = vi.fn().mockResolvedValue(undefined);
    store.serverInfo.refreshAuthenticatedSettings = vi.fn().mockResolvedValue(undefined);
    store.notifications.fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue(undefined);
    store.rooms.refresh = vi.fn().mockResolvedValue(undefined);
    store.roomDirectory.refresh = vi.fn().mockResolvedValue(undefined);
    store.adminRoomLayout.refresh = vi.fn().mockResolvedValue(undefined);

    eventBusManager.startBus(registered.id, fake as unknown as ServerConnection);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.catchUpHandlers) {
      handler('heartbeat-stalled');
    }
    await flushPromises();

    for (const handler of bus.catchUpHandlers) {
      handler('ws-reconnected');
    }
    await flushPromises();

    expect(store.notifications.fetch).toHaveBeenCalledTimes(2);
    expect(store.rooms.refresh).toHaveBeenCalledTimes(2);
    expect(consoleError).toHaveBeenCalledOnce();
  });
});
