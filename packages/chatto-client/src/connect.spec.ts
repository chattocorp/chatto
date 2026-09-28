// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import type { ConnectAPIConfig } from './api/connect.js';
import type { CurrentUser } from './api/viewer.js';

const mocks = vi.hoisted(() => ({
  discovery: vi.fn(),
  viewer: vi.fn<(config: ConnectAPIConfig) => Promise<CurrentUser>>()
}));
vi.mock('./api/server.js', async (original) => ({
  ...(await original<typeof import('./api/server.js')>()),
  getPublicServerInfo: mocks.discovery
}));
vi.mock('./api/viewer.js', async (original) => ({
  ...(await original<typeof import('./api/viewer.js')>()),
  getCurrentUserViaConnect: mocks.viewer
}));

import { connectChatto, type ChattoConnection } from './connect.js';
import { serverRegistry } from './server/registry.js';
import { eventBusManager, setRealtimeSocketFactoryForTests } from './server/realtimeTransport.js';
import { RealtimeProjectionUpdate } from './realtime/eventBus.js';

const profile = {
  name: 'Bot server',
  // A version the client does not support keeps the realtime transport closed.
  version: '0.1.0',
  welcomeMessage: null,
  description: null,
  iconUrl: null,
  bannerUrl: null,
  directRegistrationEnabled: true,
  directLoginEnabled: true
};

describe('connectChatto in Node', () => {
  let connection: ChattoConnection | undefined;

  beforeEach(() => {
    mocks.discovery.mockReset().mockResolvedValue(profile);
    mocks.viewer.mockReset().mockResolvedValue({ id: 'bot', login: 'bot' } as CurrentUser);
    setRealtimeSocketFactoryForTests(() => {
      throw new Error('no realtime in this test');
    });
  });

  afterEach(() => {
    connection?.close();
    connection = undefined;
    setRealtimeSocketFactoryForTests(null);
  });

  it('runs without browser globals and resolves the viewer', async () => {
    expect(typeof window).toBe('undefined');
    expect(typeof localStorage).toBe('undefined');
    connection = connectChatto({ serverUrl: 'https://chat.example/some/path', apiKey: 'key' });
    await expect(connection.ready()).resolves.toEqual({ viewerId: 'bot' });
    expect(serverRegistry.getServer(connection.serverId)).toMatchObject({
      url: 'https://chat.example',
      token: 'key'
    });
    expect(connection.store.accountId).toBe('bot');
  });

  it('never renews the fixed token and rejects when the server refuses it', async () => {
    mocks.viewer.mockImplementation(async (config) => {
      expect(config.renewBearerToken).toBeUndefined();
      config.onAuthenticationRequired?.('chatto.api.v1.ViewerService/GetViewer');
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });
    connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'revoked' });
    await expect(connection.ready()).rejects.toThrow('rejected the API key');
  });

  it('allows one open connection and never reuses a closed server ID', async () => {
    const first = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    expect(() => connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' })).toThrow(
      'Close the open Chatto connection'
    );
    const firstId = first.serverId;
    first.close();
    expect(serverRegistry.getServer(firstId)).toBeUndefined();
    connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    expect(connection.serverId).not.toBe(firstId);
  });

  it('rejects URLs with credentials and empty keys before registering', () => {
    const count = serverRegistry.servers.length;
    expect(() => connectChatto({ serverUrl: 'https://user:pw@chat.example', apiKey: 'k' })).toThrow(
      'without credentials'
    );
    expect(() => connectChatto({ serverUrl: 'ftp://chat.example', apiKey: 'k' })).toThrow();
    expect(() => connectChatto({ serverUrl: 'https://chat.example', apiKey: '' })).toThrow(
      'API key is required'
    );
    expect(serverRegistry.servers.length).toBe(count);
  });

  it('rejects a pending ready() when the connection closes', async () => {
    mocks.viewer.mockReturnValue(new Promise(() => {}));
    const closing = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    const ready = closing.ready();
    closing.close();
    await expect(ready).rejects.toThrow('closed');
    expect(closing.closed).toBe(true);
  });

  it('reports an ended session and keeps its token in memory', async () => {
    connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    expect(connection.sessionEnded).toBe(false);
    serverRegistry.handleAuthenticationRequired(connection.serverId);
    expect(connection.sessionEnded).toBe(true);
    expect(serverRegistry.getServer(connection.serverId)?.token).toBe('key');
  });

  it('reports a gap once for resets after the stream was connected', async () => {
    connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    await vi.waitFor(() => expect(eventBusManager.getBus(connection!.serverId)).toBeDefined());
    const bus = eventBusManager.getBus(connection.serverId)!;
    const gaps: boolean[] = [];
    connection.onReset(({ gap }) => gaps.push(gap));
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
  });

  it('leaves no timers behind after close', async () => {
    vi.useFakeTimers();
    try {
      const closing = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
      await closing.ready();
      await vi.waitFor(() => expect(eventBusManager.getBus(closing.serverId)).toBeDefined());
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
    connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await expect(connection.ready()).rejects.toThrow('Could not reach the Chatto server');
    logged.mockRestore();
  });

  it('stops waiting when the caller aborts', async () => {
    mocks.viewer.mockReturnValue(new Promise(() => {}));
    connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    const controller = new AbortController();
    const ready = connection.ready({ signal: controller.signal });
    controller.abort(new Error('stopped'));
    await expect(ready).rejects.toThrow('stopped');
  });

  it('reports disconnected and creates no connection for a closed server', async () => {
    const { serverConnectionManager } = await import('./server/serverConnection.js');
    const create = vi.spyOn(serverConnectionManager, 'getClient');
    const connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    connection.close();
    create.mockClear();
    expect(connection.status).toBe('disconnected');
    expect(create).not.toHaveBeenCalled();
    create.mockRestore();
  });
});
