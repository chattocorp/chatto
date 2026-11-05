// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { ConnectAPIConfig } from './api/connect.js';
import type { CurrentUser } from './api/viewer.js';

const mocks = vi.hoisted(() => ({
  viewer: vi.fn<(config: ConnectAPIConfig) => Promise<CurrentUser>>(),
  recoveryFails: false,
  stopPresence: vi.fn(),
  sendPresenceHeartbeats: vi.fn()
}));
vi.mock('./server/presenceTracking.js', async (original) => ({
  ...(await original<typeof import('./server/presenceTracking.js')>()),
  sendPresenceHeartbeats: mocks.sendPresenceHeartbeats
}));
vi.mock('./server/serverRecovery.js', async (original) => {
  const actual = await original<typeof import('./server/serverRecovery.js')>();
  return {
    startServerRecovery: (...args: Parameters<typeof actual.startServerRecovery>) => {
      if (mocks.recoveryFails) throw new Error('recovery could not start');
      return actual.startServerRecovery(...args);
    }
  };
});
vi.mock('./api/server.js', async (original) => ({
  ...(await original<typeof import('./api/server.js')>()),
  getPublicServerInfo: vi.fn(async () => ({ name: 'Chat', version: '0.5.0' }))
}));
vi.mock('./api/viewer.js', async (original) => ({
  ...(await original<typeof import('./api/viewer.js')>()),
  getCurrentUserViaConnect: mocks.viewer
}));

import { emptyServerSession as emptyNativeSession } from './server/sessions.js';
import { createClient, type ChattoClient } from './client.js';
import { RealtimeProjectionUpdate } from './realtime/eventBus.js';
import { setRealtimeSocketFactoryForTests } from './server/realtimeTransport.js';
import { inertRealtimeSocket } from './testing/inertSocket.js';

const clients: ChattoClient[] = [];
/** Create a client that the test closes afterwards. */
function client(...args: Parameters<typeof createClient>) {
  const created = createClient(...args);
  clients.push(created);
  return created;
}

beforeEach(() => {
  // Each server answers with the viewer of the key that the request carries.
  mocks.viewer.mockReset().mockImplementation(async (config) => {
    const id = `${config.bearerToken}-viewer`;
    return { id, login: id } as CurrentUser;
  });
  setRealtimeSocketFactoryForTests(inertRealtimeSocket);
});

afterEach(() => {
  for (const created of clients.splice(0)) created.close();
  setRealtimeSocketFactoryForTests(null);
});

describe('isolated clients', () => {
  it('keep servers, viewers, and events apart', async () => {
    const eu = client().connect({ serverUrl: 'https://eu.example', apiKey: 'eu' });
    const us = client().connect({ serverUrl: 'https://us.example', apiKey: 'us' });
    await expect(eu.ready()).resolves.toEqual({ viewerId: 'eu-viewer' });
    await expect(us.ready()).resolves.toEqual({ viewerId: 'us-viewer' });
    expect(clients[0]!.registry.servers.map((server) => server.url)).toEqual([
      'https://eu.example'
    ]);
    expect(clients[1]!.registry.servers.map((server) => server.url)).toEqual([
      'https://us.example'
    ]);

    const received = { eu: [] as string[], us: [] as string[] };
    eu.onEvent((event) => received.eu.push(event.id));
    us.onEvent((event) => received.us.push(event.id));
    await vi.waitFor(() => expect(clients[0]!.realtime.getBus(eu.serverId)).toBeDefined());
    expect(clients[1]!.realtime.getBus(eu.serverId)).toBeUndefined();
    clients[0]!.realtime.getBus(eu.serverId)!.publish(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          id: 'eu-event',
          actorId: 'human',
          event: {
            case: 'messagePosted',
            value: { roomId: 'room', roomKind: RoomKind.DM, bodyPlaintext: 'hi' }
          }
        })
      })
    );
    expect(received).toEqual({ eu: ['eu-event'], us: [] });

    eu.close();
    expect(us.closed).toBe(false);
    expect(us.viewerId).toBe('us-viewer');
  });

  it('keep several connections of one client apart, also to the same server', async () => {
    const shared = client();
    const first = shared.connect({ serverUrl: 'https://chat.example', apiKey: 'first' });
    const second = shared.connect({ serverUrl: 'https://chat.example', apiKey: 'second' });
    await expect(first.ready()).resolves.toEqual({ viewerId: 'first-viewer' });
    await expect(second.ready()).resolves.toEqual({ viewerId: 'second-viewer' });
    expect(first.serverId).not.toBe(second.serverId);
    first.close();
    expect(second.viewerId).toBe('second-viewer');
  });

  it('return one server type for connected and registered servers', async () => {
    const shared = client();
    const connected = shared.connect({ serverUrl: 'https://chat.example', apiKey: 'first' });
    expect(shared.server(connected.serverId)).toBe(connected);
    await connected.ready();
    // The server is the store: its state and requests are one object.
    expect(connected.accountId).toBe('first-viewer');

    connected.close();
    expect(connected.closed).toBe(true);
    expect(shared.server(connected.serverId)).toBeUndefined();
  });

  it('keep every connection of a bot client live', async () => {
    const sockets: string[] = [];
    setRealtimeSocketFactoryForTests((url) => {
      sockets.push(new URL(url).host);
      return inertRealtimeSocket();
    });
    const bots = client();
    const first = bots.connect({ serverUrl: 'https://one.example', apiKey: 'one' });
    const second = bots.connect({ serverUrl: 'https://two.example', apiKey: 'two' });
    await Promise.all([first.ready(), second.ready()]);
    await vi.waitFor(() =>
      expect(new Set(sockets)).toEqual(new Set(['one.example', 'two.example']))
    );
  });

  it('keeps as many recently used threads as keepRecent allows', async () => {
    const bots = client({ keepRecent: { threads: 1 } });
    const server = bots.connect({ serverUrl: 'https://bot.example', apiKey: 'bot' });
    await server.ready();
    const first = server.rooms.thread('room', 'one');
    const second = server.rooms.thread('room', 'two');
    expect(first.isDisposed).toBe(true);
    expect(second.isDisposed).toBe(false);
  });

  it('reports presence of a server only after reportPresence()', async () => {
    mocks.sendPresenceHeartbeats.mockReset().mockReturnValue(mocks.stopPresence);
    mocks.stopPresence.mockReset();
    const bots = client();
    const quiet = bots.connect({ serverUrl: 'https://quiet.example', apiKey: 'quiet' });
    await quiet.ready();
    expect(mocks.sendPresenceHeartbeats).not.toHaveBeenCalled();

    const present = bots.connect({ serverUrl: 'https://present.example', apiKey: 'present' });
    present.reportPresence();
    present.reportPresence();
    // Heartbeats start only after the server accepted the key, once.
    expect(mocks.sendPresenceHeartbeats).not.toHaveBeenCalled();
    await present.ready();
    await vi.waitFor(() => expect(mocks.sendPresenceHeartbeats).toHaveBeenCalledOnce());
    expect(mocks.stopPresence).not.toHaveBeenCalled();
    present.close();
    expect(mocks.stopPresence).toHaveBeenCalledOnce();
  });

  it('starts reporting presence after a failed first attempt succeeds later', async () => {
    vi.useFakeTimers();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      mocks.sendPresenceHeartbeats.mockReset().mockReturnValue(mocks.stopPresence);
      mocks.viewer.mockRejectedValueOnce(new TypeError('fetch failed'));
      const bots = client();
      const server = bots.connect({ serverUrl: 'https://late.example', apiKey: 'late' });
      server.reportPresence();
      await vi.advanceTimersByTimeAsync(500);
      expect(mocks.sendPresenceHeartbeats).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(30_000);
      await server.ready();
      await vi.waitFor(() => expect(mocks.sendPresenceHeartbeats).toHaveBeenCalledOnce());
    } finally {
      logged.mockRestore();
      vi.useRealTimers();
    }
  });

  it('rejects keepRecent values that are not positive integers', () => {
    expect(() => client({ keepRecent: { rooms: 0 } })).toThrow(
      'keepRecent.rooms must be a positive integer'
    );
    expect(() => client({ keepRecent: { threads: 1.5 } })).toThrow(
      'keepRecent.threads must be a positive integer'
    );
  });

  it('keep connections live in a client with one selected live server', async () => {
    const app = client({ liveServers: 'selected' });
    const connection = app.connect({ serverUrl: 'https://bot.example', apiKey: 'bot' });
    await connection.ready();
    await vi.waitFor(() => expect(app.realtime.getBus(connection.serverId)).toBeDefined());
    await new Promise((resolve) => setTimeout(resolve, 20));
    // A polled server's transport is dormant between catch-ups; a live one connects.
    expect(connection.connection.status).toBe('connecting');
  });
});

describe('client lifecycle', () => {
  it('stops its timers when the last connection closes, and cannot be used after close', async () => {
    vi.useFakeTimers();
    try {
      const bots = client();
      const connection = bots.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await connection.ready();
      await vi.waitFor(() => expect(bots.realtime.getBus(connection.serverId)).toBeDefined());
      await Promise.resolve();
      connection.close();
      expect(vi.getTimerCount()).toBe(0);
      bots.close();
      expect(() => bots.connect({ serverUrl: 'https://chat.example', apiKey: 'key' })).toThrow(
        'closed'
      );
      expect(() => bots.start()).toThrow('closed');
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('runs the runtime until every start has ended', () => {
    vi.useFakeTimers();
    try {
      const app = client({ liveServers: 'selected' });
      app.start();
      app.start();
      expect(vi.getTimerCount()).toBeGreaterThan(0);
      app.stop();
      expect(vi.getTimerCount()).toBeGreaterThan(0);
      app.stop();
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('leaves nothing behind when a connection cannot start the runtime', async () => {
    vi.useFakeTimers();
    try {
      const bots = client();
      mocks.recoveryFails = true;
      try {
        expect(() => bots.connect({ serverUrl: 'https://chat.example', apiKey: 'key' })).toThrow(
          'recovery could not start'
        );
      } finally {
        mocks.recoveryFails = false;
      }
      expect(bots.registry.servers).toEqual([]);
      const connection = bots.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await connection.ready();
      connection.close();
      // No connection of the failed attempt keeps the runtime and its timers.
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('closes every connection with the client', async () => {
    const bots = client();
    const connection = bots.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    bots.close();
    expect(connection.closed).toBe(true);
    expect(bots.registry.servers).toEqual([]);
  });

  it('allows one device-storage client and one origin-server client at a time', () => {
    const device = client({ storage: 'device' });
    expect(() => createClient({ storage: 'device' })).toThrow('device storage');
    const origin = client({ originServer: true });
    expect(() => createClient({ originServer: true })).toThrow('origin server');
    device.close();
    origin.close();
    client({ storage: 'device', originServer: true });
  });

  it('rejects server URLs with credentials and empty keys before registering', () => {
    const bots = client();
    expect(() => bots.connect({ serverUrl: 'https://user:pw@chat.example', apiKey: 'k' })).toThrow(
      'without credentials'
    );
    expect(() => bots.connect({ serverUrl: 'ftp://chat.example', apiKey: 'k' })).toThrow();
    expect(() => bots.connect({ serverUrl: 'https://chat.example', apiKey: '' })).toThrow(
      'API key is required'
    );
    expect(bots.registry.servers).toEqual([]);
  });
});

describe('native durable persistence', () => {
  function persistence() {
    let servers: import('./server/registry.js').RegisteredServer[] = [];
    const authentication = new Map<
      string,
      import('./server/registry.js').ServerAuthentication | null
    >();
    let writable = true;
    const storage: import('./server/registry.js').ServerRegistryPersistence = {
      shared: false,
      readServers: () => structuredClone(servers),
      readAuthentication: (id) => authentication.get(id),
      writeAuthentication: (id, value) => {
        if (!writable) return false;
        authentication.set(id, { ...value });
        return true;
      },
      writeServers: (value) => {
        servers = structuredClone(value);
      }
    };
    return {
      storage,
      authentication,
      setWritable: (value: boolean) => {
        writable = value;
      }
    };
  }
  function add(owner: ChattoClient) {
    owner.registry.addServer(
      {
        id: 'native-fixture',
        url: 'https://native.example',
        name: 'Fixture',
        iconUrl: null,
        addedAt: 1
      },
      {
        ...emptyNativeSession(),
        token: 'old-access',
        refreshToken: 'old-refresh',
        oauthClientId: 'native-client',
        accessTokenExpiresAt: Date.now() + 300_000,
        refreshTokenExpiresAt: Date.now() + 600_000
      }
    );
  }
  it('restores latest authentication rather than a stale combined snapshot', () => {
    const disk = persistence();
    const first = client({ persistence: disk.storage });
    add(first);
    disk.authentication.set('native-fixture', {
      ...disk.authentication.get('native-fixture')!,
      refreshRequestId: 'durable-request-id'
    });
    first.close();
    const second = client({ persistence: disk.storage });
    expect(second.registry.getServer('native-fixture')?.refreshRequestId).toBe(
      'durable-request-id'
    );
  });
  it('does not combine host persistence with browser device storage', () => {
    expect(() => createClient({ storage: 'device', persistence: persistence().storage })).toThrow(
      'Choose browser'
    );
  });
  it('rejects a malformed restored catalogue', () => {
    const disk = persistence();
    disk.storage.readServers = () => JSON.parse('[{"id":"broken"}]');
    expect(() => createClient({ persistence: disk.storage })).toThrow('Invalid persisted');
  });
  it('keeps a durable tombstone signed out after restart', () => {
    const disk = persistence();
    const first = client({ persistence: disk.storage });
    add(first);
    disk.authentication.set('native-fixture', null);
    first.close();
    const second = client({ persistence: disk.storage });
    expect(second.registry.getServer('native-fixture')?.token).toBeNull();
  });
  it('retries a lost refresh across restart with the same durable request ID', async () => {
    const disk = persistence();
    let first = client({ persistence: disk.storage });
    add(first);
    const ids: string[] = [];
    const fetch = vi.fn(async (_url: unknown, options: RequestInit) => {
      const body = JSON.parse(String(options.body));
      ids.push(body.refresh_request_id);
      expect(disk.authentication.get('native-fixture')?.refreshRequestId).toBe(
        body.refresh_request_id
      );
      expect(options).toMatchObject({ credentials: 'omit', redirect: 'error' });
      if (ids.length === 1) throw new Error('lost response');
      return Response.json({
        access_token: 'new-access',
        refresh_token: 'new-refresh',
        expires_in: 300,
        refresh_token_expires_in: 600
      });
    });
    vi.stubGlobal('fetch', fetch);
    try {
      await expect(
        first.registry.renewServerAuthentication('native-fixture', true)
      ).rejects.toThrow('lost response');
      first.close();
      first = client({ persistence: disk.storage });
      await expect(first.registry.renewServerAuthentication('native-fixture', true)).resolves.toBe(
        'new-access'
      );
      expect(ids[0]).toBe(ids[1]);
      expect(disk.authentication.get('native-fixture')?.refreshToken).toBe('new-refresh');
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it('refuses a replacement token when its durable write fails', async () => {
    const disk = persistence();
    const owner = client({ persistence: disk.storage });
    add(owner);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        disk.setWritable(false);
        return Response.json({
          access_token: 'new-access',
          refresh_token: 'new-refresh',
          expires_in: 300,
          refresh_token_expires_in: 600
        });
      })
    );
    try {
      await expect(
        owner.registry.renewServerAuthentication('native-fixture', true)
      ).rejects.toThrow('Unable to persist renewed');
      expect(disk.authentication.get('native-fixture')?.token).toBe('old-access');
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
