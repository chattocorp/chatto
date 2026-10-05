// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { ConnectAPIConfig } from './api/connect.js';
import type { CurrentUser } from './api/viewer.js';

const mocks = vi.hoisted(() => ({
  viewer: vi.fn<(config: ConnectAPIConfig) => Promise<CurrentUser>>(),
  recoveryFails: false
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
