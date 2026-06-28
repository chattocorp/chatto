import { describe, it, expect, beforeEach, vi } from 'vitest';

const { mockHandleAuthenticationRequired, mockServers } = vi.hoisted(() => ({
  mockHandleAuthenticationRequired: vi.fn(),
  mockServers: new Map<string, { id: string; url: string; token: string | null }>()
}));

vi.mock('./registry.svelte', () => ({
  serverRegistry: {
    getServer: (id: string) => mockServers.get(id),
    isOriginServer: (id: string) => mockServers.get(id)?.url === window.location.origin,
    get originServer() {
      return [...mockServers.values()].find((s) => s.url === window.location.origin);
    },
    handleAuthenticationRequired: mockHandleAuthenticationRequired
  }
}));

import { httpToWsUrl, ServerConnection, type ServerConnectionConfig } from './serverConnection.svelte';

function makeConfig(overrides: Partial<ServerConnectionConfig> = {}): ServerConnectionConfig {
  return {
    serverUrl: '/',
    token: null,
    ...overrides
  };
}

describe('httpToWsUrl', () => {
  it('converts http to ws', () => {
    expect(httpToWsUrl('http://localhost:4000/api/realtime')).toBe(
      'ws://localhost:4000/api/realtime'
    );
  });

  it('converts https to wss', () => {
    expect(httpToWsUrl('https://chat.example.com/api/realtime')).toBe(
      'wss://chat.example.com/api/realtime'
    );
  });

  it('leaves non-http URLs unchanged', () => {
    expect(httpToWsUrl('/api/realtime')).toBe('/api/realtime');
  });
});

describe('ServerConnection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockServers.clear();
  });

  it('derives origin Connect and realtime endpoints', () => {
    const client = new ServerConnection(makeConfig({ serverUrl: '/' }));

    expect(client.connectBaseUrl).toBe(`${window.location.origin}/api/connect`);
    expect(client.realtimeUrl).toBe(httpToWsUrl(`${window.location.origin}/api/realtime`));
    client.dispose();
  });

  it('derives remote Connect and realtime endpoints', () => {
    const client = new ServerConnection(
      makeConfig({ serverUrl: 'https://remote.example.com', token: 'my-token' })
    );

    expect(client.connectBaseUrl).toBe('https://remote.example.com/api/connect');
    expect(client.realtimeUrl).toBe('wss://remote.example.com/api/realtime');
    expect(client.bearerToken).toBe('my-token');
    client.dispose();
  });

  it('starts with status "connecting" and reconnectCount 0', () => {
    const client = new ServerConnection(makeConfig());
    expect(client.status).toBe('connecting');
    expect(client.reconnectCount).toBe(0);
    client.dispose();
  });

  it('tracks realtime connection status and failed attempts', () => {
    const client = new ServerConnection(makeConfig());

    client.setRealtimeConnectionStatus('connected');
    client.setRealtimeConnectionStatus('disconnected', 6);
    expect(client.status).toBe('disconnected');
    expect(client.showConnectionLostIcon).toBe(true);
    expect(client.showConnectionLostBanner).toBe(true);

    client.setRealtimeConnectionStatus('connecting', 6);
    client.setRealtimeConnectionStatus('connected');
    expect(client.status).toBe('connected');
    expect(client.reconnectCount).toBe(1);
    expect(client.showConnectionLostBanner).toBe(false);
    client.dispose();
  });

  it('forces reconnect through the registered realtime handler', () => {
    const client = new ServerConnection(makeConfig());
    const reconnect = vi.fn();

    client.setRealtimeConnectionStatus('connected');
    client.registerRealtimeReconnect(reconnect);
    client.forceReconnect('test');

    expect(reconnect).toHaveBeenCalledWith('test');
    client.dispose();
  });

  it('unregisters realtime reconnect handlers', () => {
    const client = new ServerConnection(makeConfig());
    const reconnect = vi.fn();
    const unregister = client.registerRealtimeReconnect(reconnect);

    client.setRealtimeConnectionStatus('connected');
    unregister();
    client.forceReconnect('test');

    expect(reconnect).not.toHaveBeenCalled();
    client.dispose();
  });

  it('is a no-op while a connection attempt is already in flight', () => {
    const client = new ServerConnection(makeConfig());
    const reconnect = vi.fn();

    client.registerRealtimeReconnect(reconnect);
    client.forceReconnect('first');
    client.forceReconnect('second');

    expect(reconnect).not.toHaveBeenCalled();
    client.dispose();
  });

  it('clears registered server auth on realtime authentication-required signals', () => {
    const client = new ServerConnection(makeConfig({ token: 'my-token', serverId: 'remote-1' }));

    client.handleAuthenticationRequired();

    expect(mockHandleAuthenticationRequired).toHaveBeenCalledWith('remote-1');
    client.dispose();
  });
});

describe('ServerConnectionManager', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockServers.clear();
  });

  it('exports serverConnectionManager', async () => {
    const mod = await import('./serverConnection.svelte');
    expect(mod.serverConnectionManager).toBeDefined();
  });

  it('originClient uses relative URL', async () => {
    const mod = await import('./serverConnection.svelte');
    expect(mod.serverConnectionManager.originClient).toBeDefined();
    expect(mod.serverConnectionManager.originClient.status).toBe('connecting');
  });

  it('getClient returns originClient for home instances', async () => {
    const mod = await import('./serverConnection.svelte');
    mockServers.set('my-home', {
      id: 'my-home',
      url: window.location.origin,
      token: 'origin-token'
    });

    const client = mod.serverConnectionManager.getClient('my-home');
    expect(client).toBe(mod.serverConnectionManager.originClient);
  });

  it('originClient uses the registered origin token when present', async () => {
    const mod = await import('./serverConnection.svelte');
    mockServers.set('my-home', {
      id: 'my-home',
      url: window.location.origin,
      token: 'origin-token'
    });

    mod.serverConnectionManager.destroyClient('my-home');
    expect(mod.serverConnectionManager.originClient.bearerToken).toBe('origin-token');
  });

  it('getClient throws for unknown instance IDs', async () => {
    const mod = await import('./serverConnection.svelte');
    expect(() => mod.serverConnectionManager.getClient('nonexistent')).toThrow(
      'Server "nonexistent" not found in registry'
    );
  });

  it('getClient creates and caches remote clients', async () => {
    const mod = await import('./serverConnection.svelte');
    mockServers.set('remote-1', {
      id: 'remote-1',
      url: 'https://remote.example.com',
      token: 'remote-token'
    });

    const client1 = mod.serverConnectionManager.getClient('remote-1');
    const client2 = mod.serverConnectionManager.getClient('remote-1');
    expect(client1).toBe(client2);
    expect(client1).not.toBe(mod.serverConnectionManager.originClient);
    expect(client1.connectBaseUrl).toBe('https://remote.example.com/api/connect');
  });

  it('destroyClient disposes and removes remote clients', async () => {
    const mod = await import('./serverConnection.svelte');
    mockServers.set('remote-2', {
      id: 'remote-2',
      url: 'https://other.example.com',
      token: 'token-2'
    });

    const oldClient = mod.serverConnectionManager.getClient('remote-2');

    expect(mod.serverConnectionManager.destroyClient('remote-2')).toBe(true);

    const newClient = mod.serverConnectionManager.getClient('remote-2');
    expect(newClient).toBeDefined();
    expect(newClient).not.toBe(oldClient);
  });

  it('destroyClient returns false for nonexistent clients', async () => {
    const mod = await import('./serverConnection.svelte');
    expect(mod.serverConnectionManager.destroyClient('nope')).toBe(false);
  });
});
