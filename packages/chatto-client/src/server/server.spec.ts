// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import type { ConnectAPIConfig } from '../api/connect.js';
import type { CurrentUser } from '../api/viewer.js';

const mocks = vi.hoisted(() => ({
  discovery: vi.fn(),
  viewer: vi.fn<(config: ConnectAPIConfig) => Promise<CurrentUser>>()
}));
vi.mock('../api/server.js', async (original) => ({
  ...(await original<typeof import('../api/server.js')>()),
  getPublicServerInfo: mocks.discovery
}));
vi.mock('../api/viewer.js', async (original) => ({
  ...(await original<typeof import('../api/viewer.js')>()),
  getCurrentUserViaConnect: mocks.viewer
}));

import { createClient, type ChattoClient } from '../client.js';
import type { Server } from './server.js';
import { emptyServerSession } from './sessions.js';
import { setRealtimeSocketFactoryForTests } from './realtimeTransport.js';
import { RealtimeProjectionUpdate } from '../realtime/eventBus.js';
import { RealtimeResourceUpdate } from '../api/realtimeResources.js';
import { batch } from '../reactivity/index.js';
import { ListRoomsResponse, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { inertRealtimeSocket } from '../testing/inertSocket.js';

const profile = {
  name: 'Bot server',
  version: '0.5.0',
  welcomeMessage: null,
  description: null,
  iconUrl: null,
  bannerUrl: null,
  directRegistrationEnabled: true,
  directLoginEnabled: true
};

describe('connections in Node', () => {
  let client: ChattoClient;
  let connection: Server | undefined;

  beforeEach(() => {
    client = createClient();
    mocks.discovery.mockReset().mockResolvedValue(profile);
    mocks.viewer.mockReset().mockResolvedValue({ id: 'bot', login: 'bot' } as CurrentUser);
    setRealtimeSocketFactoryForTests(inertRealtimeSocket);
  });

  afterEach(() => {
    connection?.close();
    connection = undefined;
    client.close();
    setRealtimeSocketFactoryForTests(null);
  });

  it('runs without browser globals and resolves the viewer', async () => {
    expect(typeof window).toBe('undefined');
    expect(typeof localStorage).toBe('undefined');
    connection = client.connect({ serverUrl: 'https://chat.example/some/path', apiKey: 'key' });
    await expect(connection.ready()).resolves.toEqual({ viewerId: 'bot' });
    expect(client.registry.getServer(connection.serverId)).toMatchObject({
      url: 'https://chat.example',
      token: 'key'
    });
    expect(connection.accountId).toBe('bot');
  });

  it('never renews the fixed token and rejects when the server refuses it', async () => {
    mocks.viewer.mockImplementation(async (config) => {
      expect(config.renewBearerToken).toBeUndefined();
      config.onAuthenticationRequired?.('chatto.api.v1.ViewerService/GetViewer');
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'revoked' });
    await expect(connection.ready()).rejects.toThrow('rejected the API key');
  });

  it('reports a rejected key although discovery failed too', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const warned = vi.spyOn(console, 'warn').mockImplementation(() => {});
    mocks.discovery.mockRejectedValue(new TypeError('fetch failed'));
    mocks.viewer.mockImplementation(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'revoked' });
    await expect(connection.ready()).rejects.toThrow('rejected the API key');
    logged.mockRestore();
    warned.mockRestore();
  });

  it('holds several connections and never reuses a server ID', async () => {
    const first = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    const second = client.connect({ serverUrl: 'https://chat.example', apiKey: 'other' });
    expect(second.serverId).not.toBe(first.serverId);
    await expect(Promise.all([first.ready(), second.ready()])).resolves.toHaveLength(2);
    const firstId = first.serverId;
    first.close();
    second.close();
    expect(client.registry.getServer(firstId)).toBeUndefined();
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    expect([firstId, second.serverId]).not.toContain(connection.serverId);
  });

  it('rejects URLs with credentials and empty keys before registering', () => {
    const count = client.registry.servers.length;
    expect(() =>
      client.connect({ serverUrl: 'https://user:pw@chat.example', apiKey: 'k' })
    ).toThrow('without credentials');
    expect(() => client.connect({ serverUrl: 'ftp://chat.example', apiKey: 'k' })).toThrow();
    expect(() => client.connect({ serverUrl: 'https://chat.example', apiKey: '' })).toThrow(
      'API key is required'
    );
    expect(client.registry.servers.length).toBe(count);
  });

  it('rejects a pending ready() when the connection closes', async () => {
    mocks.viewer.mockReturnValue(new Promise(() => {}));
    const closing = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    const ready = closing.ready();
    closing.close();
    await expect(ready).rejects.toThrow('closed');
    expect(closing.closed).toBe(true);
  });

  it('reports an ended session and keeps its token in memory', async () => {
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    expect(connection.sessionEnded).toBe(false);
    client.registry.handleAuthenticationRequired(connection.serverId);
    expect(connection.sessionEnded).toBe(true);
    expect(client.registry.getServer(connection.serverId)?.token).toBe('key');
  });

  it('reports a gap once for resets after the stream was connected', async () => {
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    await vi.waitFor(() => expect(client.realtime.getBus(connection!.serverId)).toBeDefined());
    const bus = client.realtime.getBus(connection.serverId)!;
    const gaps: boolean[] = [];
    connection.onSnapshot(({ gap }) => gaps.push(gap));
    const reset = () => bus.publish(new RealtimeProjectionUpdate({ reset: true }));

    reset(); // initial snapshot
    connection.connection.setRealtimeConnectionStatus('connected');
    reset(); // a resync after a connected stream
    connection.connection.setRealtimeConnectionStatus('connecting');
    reset(); // its snapshot, before the next connection
    connection.connection.setRealtimeConnectionStatus('connected');
    connection.connection.setRealtimeConnectionStatus('connecting');
    reset(); // a snapshot that replaced a stream that could not resume

    expect(gaps).toEqual([false, true, false, true]);

    // A snapshot whose catch-up events were delivered before the stream
    // failed, replaced before the stream connected.
    bus.publish(new RealtimeProjectionUpdate({ event: new RealtimeEvent({ id: 'catch-up' }) }));
    reset();
    expect(gaps.at(-1)).toBe(true);
  });

  it('keeps delivering an event to later listeners when one listener throws', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await vi.waitFor(() => expect(client.realtime.getBus(connection!.serverId)).toBeDefined());
    const received: string[] = [];
    connection.onEvent(() => {
      throw new Error('listener failed');
    });
    connection.onEvent((event) => received.push(event.id));
    client.realtime
      .getBus(connection.serverId)!
      .publish(new RealtimeProjectionUpdate({ event: new RealtimeEvent({ id: 'e1' }) }));
    expect(received).toEqual(['e1']);
    logged.mockRestore();
  });

  it('reports a reset after its snapshot applied', async () => {
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    await vi.waitFor(() => expect(client.realtime.getBus(connection!.serverId)).toBeDefined());
    const bus = client.realtime.getBus(connection.serverId)!;
    const roomsAtReset: number[] = [];
    connection.onSnapshot(() => roomsAtReset.push(connection!.projection.rooms.size));
    batch(() => {
      bus.publish(new RealtimeProjectionUpdate({ reset: true }));
      bus.publish(
        new RealtimeProjectionUpdate({
          resource: new RealtimeResourceUpdate({
            resource: {
              case: 'rooms',
              value: new ListRoomsResponse({
                rooms: [new RoomWithViewerState({ room: { id: 'R1', name: 'general' } })]
              })
            },
            replace: true
          })
        })
      );
    });
    expect(roomsAtReset).toEqual([1]);
  });

  it('leaves no timers behind after close', async () => {
    vi.useFakeTimers();
    try {
      const closing = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await closing.ready();
      await vi.waitFor(() => expect(client.realtime.getBus(closing.serverId)).toBeDefined());
      await Promise.resolve();
      closing.close();
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('rejects ready() when the viewer read fails', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    mocks.viewer.mockRejectedValue(new TypeError('fetch failed'));
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await expect(connection.ready()).rejects.toThrow('Could not load the viewer');
    logged.mockRestore();
  });

  it('keeps waiting when a viewer read is superseded instead of failing', async () => {
    let resolveViewer!: (user: CurrentUser) => void;
    mocks.viewer.mockReturnValueOnce(new Promise((resolve) => (resolveViewer = resolve)));
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    let settled = false;
    const ready = connection.ready().finally(() => (settled = true));
    await vi.waitFor(() => expect(mocks.viewer).toHaveBeenCalled());
    connection.currentUser.invalidateVerification();
    resolveViewer({ id: 'bot', login: 'bot' } as CurrentUser);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
    await connection.currentUser.load();
    await expect(ready).resolves.toEqual({ viewerId: 'bot' });
  });

  it('resolves when the viewer loads although discovery failed', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    mocks.discovery.mockRejectedValue(new TypeError('discovery blocked'));
    mocks.viewer.mockImplementation(
      () =>
        new Promise((resolve) =>
          setTimeout(() => resolve({ id: 'bot', login: 'bot' } as CurrentUser), 10)
        )
    );
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await expect(connection.ready()).resolves.toEqual({ viewerId: 'bot' });
    logged.mockRestore();
  });

  it('reports each failed discovery attempt to a waiting ready()', async () => {
    vi.useFakeTimers();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const warned = vi.spyOn(console, 'warn').mockImplementation(() => {});
    try {
      mocks.discovery.mockRejectedValue(new TypeError('fetch failed'));
      mocks.viewer.mockRejectedValue(new TypeError('fetch failed'));
      connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await expect(connection.ready()).rejects.toThrow();
      const retry = connection.ready();
      retry.catch(() => {});
      await vi.advanceTimersByTimeAsync(30_000);
      await expect(retry).rejects.toThrow('Could not reach the Chatto server');
      mocks.discovery.mockResolvedValue(profile);
      mocks.viewer.mockResolvedValue({ id: 'bot', login: 'bot' } as CurrentUser);
      const recovered = connection.ready();
      await vi.advanceTimersByTimeAsync(30_000);
      await expect(recovered).resolves.toEqual({ viewerId: 'bot' });
    } finally {
      logged.mockRestore();
      warned.mockRestore();
      vi.useRealTimers();
    }
  });

  it('waits for the next recovery attempt when ready() is called again after a failure', async () => {
    vi.useFakeTimers();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      mocks.viewer.mockRejectedValueOnce(new TypeError('fetch failed'));
      connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await expect(connection.ready()).rejects.toThrow('Could not load the viewer');
      let settled = false;
      const retry = connection.ready().finally(() => (settled = true));
      await vi.advanceTimersByTimeAsync(500);
      expect(settled).toBe(false);
      await vi.advanceTimersByTimeAsync(30_000);
      await expect(retry).resolves.toEqual({ viewerId: 'bot' });
    } finally {
      logged.mockRestore();
      vi.useRealTimers();
    }
  });

  it('fails service requests at close, but not at a privacy reset', async () => {
    const { ViewerService } = await import('@chatto/api-types/api/v1/viewer_connect');
    const responses: (() => void)[] = [];
    const requestSignals: AbortSignal[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init: RequestInit) => {
        requestSignals.push(init.signal!);
        return new Promise<Response>((resolve) =>
          responses.push(() =>
            resolve(
              new Response(new Uint8Array(), {
                headers: { 'Content-Type': 'application/proto' }
              })
            )
          )
        );
      })
    );
    try {
      connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await connection.ready();
      const viewer = connection.service(ViewerService);

      const beforeReset = viewer.getViewer({});
      await vi.waitFor(() => expect(responses).toHaveLength(1));
      connection.connection.invalidatePrivateData();
      responses[0]!();
      await expect(beforeReset).resolves.toBeDefined();

      const beforeClose = viewer.getViewer({});
      await vi.waitFor(() => expect(responses).toHaveLength(2));
      connection.close();
      // close() cancels the request in flight, so no open request outlives it.
      expect(requestSignals[1]!.aborted).toBe(true);
      responses[1]!();
      await expect(beforeClose).rejects.toThrow();

      await expect(viewer.getViewer({})).rejects.toThrow('server is closed');
      expect(responses).toHaveLength(2);
      expect(() => connection!.service(ViewerService)).toThrow('server is closed');
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('rejects ready() for a server release without a supported realtime projection', async () => {
    mocks.discovery.mockResolvedValue({ ...profile, version: '0.1.0' });
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await expect(connection.ready()).rejects.toThrow('version is not supported');
  });

  it('fails a response across a privacy reset for a server without a fixed token', async () => {
    const { ViewerService } = await import('@chatto/api-types/api/v1/viewer_connect');
    const responses: (() => void)[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) =>
            responses.push(() =>
              resolve(
                new Response(new Uint8Array(), {
                  headers: { 'Content-Type': 'application/proto' }
                })
              )
            )
          )
      )
    );
    try {
      client.registry.addServer(
        { id: 'renewable', url: 'https://chat.example', name: 'chat', iconUrl: null, addedAt: 1 },
        { ...emptyServerSession(), token: 'session-token' }
      );
      const server = client.registry.getStore('renewable');
      const pending = server.service(ViewerService).getViewer({});
      await vi.waitFor(() => expect(responses.length).toBeGreaterThan(0));
      server.connection.invalidatePrivateData();
      for (const respond of responses) respond();
      await expect(pending).rejects.toThrow();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('leaves nothing behind when setup fails, so a later connect works', async () => {
    // Fail only the connection's own bus subscription.
    const original = client.realtime.getBus.bind(client.realtime);
    const getBus = vi.spyOn(client.realtime, 'getBus').mockImplementation((id) => {
      if (new Error().stack?.includes('subscribeToBus')) throw new Error('bus unavailable');
      return original(id);
    });
    const before = client.registry.servers.length;
    try {
      expect(() => client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' })).toThrow(
        'bus unavailable'
      );
    } finally {
      getBus.mockRestore();
    }
    expect(client.registry.servers).toHaveLength(before);
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await expect(connection.ready()).resolves.toEqual({ viewerId: 'bot' });
  });

  it('reports an unsupported release that a later discovery attempt finds', async () => {
    vi.useFakeTimers();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      mocks.discovery.mockRejectedValueOnce(new TypeError('fetch failed'));
      mocks.discovery.mockResolvedValue({ ...profile, version: '0.1.0' });
      mocks.viewer.mockImplementation(
        () =>
          new Promise((resolve) =>
            setTimeout(() => resolve({ id: 'bot', login: 'bot' } as CurrentUser), 10)
          )
      );
      connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      const ready = connection.ready();
      await vi.advanceTimersByTimeAsync(20);
      await expect(ready).resolves.toEqual({ viewerId: 'bot' });
      expect(connection.realtimeUnsupported).toBe(false);
      await vi.advanceTimersByTimeAsync(30_000);
      expect(connection.realtimeUnsupported).toBe(true);
    } finally {
      logged.mockRestore();
      vi.useRealTimers();
    }
  });

  it('rejects run() with the terminal condition, not the last failed attempt', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    mocks.discovery.mockResolvedValue({ ...profile, version: '0.1.0' });
    mocks.viewer.mockRejectedValue(new TypeError('fetch failed'));
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await expect(connection.run(() => {})).rejects.toThrow('version is not supported');
    logged.mockRestore();
  });

  it('stops waiting when the caller aborts', async () => {
    mocks.viewer.mockReturnValue(new Promise(() => {}));
    connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    const controller = new AbortController();
    const ready = connection.ready({ signal: controller.signal });
    controller.abort(new Error('stopped'));
    await expect(ready).rejects.toThrow('stopped');
  });

  it('reports disconnected and creates no connection for a closed server', async () => {
    const create = vi.spyOn(client.connections, 'getClient');
    const connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    connection.close();
    create.mockClear();
    expect(connection.status).toBe('disconnected');
    expect(create).not.toHaveBeenCalled();
    create.mockRestore();
  });
});
