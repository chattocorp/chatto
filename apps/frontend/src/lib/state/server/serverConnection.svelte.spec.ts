import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const {
  mockCsrfFetch,
  mockHandleAuthenticationRequired,
  mockRenewServerAuthentication,
  mockServers
} = vi.hoisted(() => ({
  mockCsrfFetch: vi.fn(),
  mockHandleAuthenticationRequired: vi.fn(),
  mockRenewServerAuthentication: vi.fn(),
  mockServers: new Map<
    string,
    {
      id: string;
      url: string;
      token: string | null;
      accessTokenExpiresAt?: number | null;
      refreshTokenExpiresAt?: number | null;
    }
  >()
}));

vi.mock('$lib/auth/csrf', () => ({ csrfFetch: mockCsrfFetch }));

vi.mock('./registry.svelte', () => ({
  serverRegistry: {
    getServer: (id: string) => mockServers.get(id),
    isOriginServer: (id: string) => mockServers.get(id)?.url === window.location.origin,
    get originServer() {
      return [...mockServers.values()].find((s) => s.url === window.location.origin);
    },
    handleAuthenticationRequired: mockHandleAuthenticationRequired,
    renewServerAuthentication: mockRenewServerAuthentication
  }
}));

import {
  httpToWsUrl,
  ServerConnection,
  type ServerConnectionConfig
} from './serverConnection.svelte';

function makeConfig(overrides: Partial<ServerConnectionConfig> = {}): ServerConnectionConfig {
  return {
    serverUrl: '/',
    token: null,
    ...overrides
  };
}

const requestLockImmediately = (async (
  name: string,
  optionsOrCallback: LockOptions | LockGrantedCallback<unknown>,
  callback?: LockGrantedCallback<unknown>
) => {
  const operation = typeof optionsOrCallback === 'function' ? optionsOrCallback : callback;
  if (!operation) throw new Error('Missing lock callback');
  return operation({ name, mode: 'exclusive' });
}) as LockManager['request'];

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
    mockCsrfFetch.mockResolvedValue(new Response(null, { status: 200 }));
    mockRenewServerAuthentication.mockImplementation(async (id: string) => {
      mockHandleAuthenticationRequired(id);
      return null;
    });
    // Real Web Locks are shared by parallel browser specs on this test origin.
    // Keep this unit spec deterministic while still exercising the lock path.
    vi.spyOn(navigator.locks, 'request').mockImplementation(requestLockImmediately);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
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

  it('starts with status "connecting"', () => {
    const client = new ServerConnection(makeConfig());
    expect(client.status).toBe('connecting');
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
    expect(client.showConnectionLostBanner).toBe(false);
    client.dispose();
  });

  it('does not present intentional dormant transport as a connection failure', () => {
    const client = new ServerConnection(makeConfig());

    client.setRealtimeConnectionStatus('dormant');

    expect(client.status).toBe('dormant');
    expect(client.isConnected).toBe(false);
    expect(client.showConnectionLostIcon).toBe(false);
    expect(client.showConnectionLostBanner).toBe(false);
    client.dispose();
  });

  it('creates each API facade once with this connection configuration', () => {
    const client = new ServerConnection(
      makeConfig({
        serverUrl: 'https://remote.example.com',
        token: 'my-token',
        serverId: 'remote-1'
      })
    );
    const factory = vi.fn((config) => ({ config }));

    const first = client.getAPI(factory);
    const second = client.getAPI(factory);

    expect(first).toBe(second);
    expect(factory).toHaveBeenCalledOnce();
    expect(factory).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: 'remote-1',
        baseUrl: 'https://remote.example.com/api/connect',
        bearerToken: 'my-token'
      })
    );
    client.dispose();
  });

  it('drops cached API facades when disposed', () => {
    const client = new ServerConnection(makeConfig());
    const factory = vi.fn(() => ({}));

    const first = client.getAPI(factory);
    client.dispose();
    const second = client.getAPI(factory);

    expect(second).not.toBe(first);
    expect(factory).toHaveBeenCalledTimes(2);
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

  it('forces reconnect on browser online events even while marked connected', () => {
    const client = new ServerConnection(makeConfig());
    const reconnect = vi.fn();

    client.setRealtimeConnectionStatus('connected');
    client.registerRealtimeReconnect(reconnect);
    window.dispatchEvent(new Event('online'));

    expect(reconnect).toHaveBeenCalledWith('network came back online');
    client.dispose();
  });

  it('replaces a connected transport after a meaningful hidden interval', () => {
    const originalVisibility = Object.getOwnPropertyDescriptor(document, 'visibilityState');
    const now = vi.spyOn(Date, 'now');
    let currentTime = 1_000;
    now.mockImplementation(() => currentTime);
    const client = new ServerConnection(makeConfig());
    const reconnect = vi.fn();
    client.setRealtimeConnectionStatus('connected');
    client.registerRealtimeReconnect(reconnect);

    try {
      Object.defineProperty(document, 'visibilityState', {
        value: 'hidden',
        configurable: true
      });
      document.dispatchEvent(new Event('visibilitychange'));
      currentTime += 29_999;
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        configurable: true
      });
      document.dispatchEvent(new Event('visibilitychange'));
      expect(reconnect).not.toHaveBeenCalled();

      Object.defineProperty(document, 'visibilityState', {
        value: 'hidden',
        configurable: true
      });
      document.dispatchEvent(new Event('visibilitychange'));
      currentTime += 30_000;
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        configurable: true
      });
      document.dispatchEvent(new Event('visibilitychange'));

      expect(reconnect).toHaveBeenCalledWith('tab visible after 30s hidden');
    } finally {
      client.dispose();
      now.mockRestore();
      if (originalVisibility) {
        Object.defineProperty(document, 'visibilityState', originalVisibility);
      }
    }
  });

  it('asks the registry to renew on realtime authentication-required signals', async () => {
    const client = new ServerConnection(makeConfig({ token: 'my-token', serverId: 'remote-1' }));

    await client.handleAuthenticationRequired();

    expect(mockRenewServerAuthentication).toHaveBeenCalledWith('remote-1', true);
    expect(mockHandleAuthenticationRequired).toHaveBeenCalledWith('remote-1');
    client.dispose();
  });

  it('renews an origin cookie through the dedicated browser endpoint', async () => {
    mockServers.set('origin', {
      id: 'origin',
      url: window.location.origin,
      token: null
    });
    const client = new ServerConnection(makeConfig({ serverId: 'origin' }));

    await expect(client.renewBrowserSession()).resolves.toBe(true);

    expect(mockCsrfFetch).toHaveBeenCalledOnce();
    expect(mockCsrfFetch).toHaveBeenCalledWith('/auth/browser/session/renew', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Chatto-Authentication-Mode': 'cookie'
      },
      body: '{}'
    });
    client.dispose();
  });

  it('coalesces concurrent origin-cookie renewal requests', async () => {
    mockServers.set('origin', {
      id: 'origin',
      url: window.location.origin,
      token: null
    });
    let finishRenewal!: (response: Response) => void;
    mockCsrfFetch.mockReturnValue(
      new Promise<Response>((resolve) => {
        finishRenewal = resolve;
      })
    );
    const client = new ServerConnection(makeConfig({ serverId: 'origin' }));

    const first = client.renewBrowserSession();
    const second = client.renewBrowserSession();
    finishRenewal(new Response(null, { status: 200 }));

    await expect(Promise.all([first, second])).resolves.toEqual([true, true]);
    expect(mockCsrfFetch).toHaveBeenCalledOnce();
    client.dispose();
  });

  it('uses the server renewal deadline without requiring realtime transport', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    mockServers.set('origin', {
      id: 'origin',
      url: window.location.origin,
      token: null
    });
    mockCsrfFetch
      .mockResolvedValueOnce(
        Response.json({ renewAfter: new Date(61_000).toISOString() }, { status: 200 })
      )
      .mockResolvedValue(
        Response.json({ renewAfter: new Date(86_461_000).toISOString() }, { status: 200 })
      );
    const client = new ServerConnection(makeConfig({ serverId: 'origin' }));

    await client.renewBrowserSession();
    await vi.advanceTimersByTimeAsync(59_999);
    expect(mockCsrfFetch).toHaveBeenCalledOnce();

    await vi.advanceTimersByTimeAsync(1);
    expect(mockCsrfFetch).toHaveBeenCalledTimes(2);
    client.dispose();
  });

  it('retries initial browser-session maintenance after a transient failure', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    mockServers.set('origin', {
      id: 'origin',
      url: window.location.origin,
      token: null
    });
    mockCsrfFetch
      .mockRejectedValueOnce(new Error('temporarily unavailable'))
      .mockResolvedValue(
        Response.json({ renewAfter: new Date(86_401_000).toISOString() }, { status: 200 })
      );
    const client = new ServerConnection(makeConfig({ serverId: 'origin' }));

    client.maintainBrowserSession();
    await vi.advanceTimersByTimeAsync(0);
    expect(mockCsrfFetch).toHaveBeenCalledOnce();

    await vi.advanceTimersByTimeAsync(60_000);
    expect(mockCsrfFetch).toHaveBeenCalledTimes(2);
    client.dispose();
  });

  it('schedules short-lived access renewal before expiry without an immediate loop', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    mockRenewServerAuthentication.mockResolvedValue('renewed-token');
    const client = new ServerConnection(
      makeConfig({
        token: 'my-token',
        serverId: 'remote-1',
        accessTokenExpiresAt: 31_000
      })
    );

    await vi.advanceTimersByTimeAsync(23_999);
    expect(mockRenewServerAuthentication).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(mockRenewServerAuthentication).toHaveBeenCalledOnce();
    expect(mockRenewServerAuthentication).toHaveBeenCalledWith('remote-1', true);
    client.dispose();
  });

  it('renews before the current session window ends', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    mockRenewServerAuthentication.mockResolvedValue('renewed-token');
    const currentWindowExpiry = 31_000;
    const client = new ServerConnection(
      makeConfig({
        token: 'my-token',
        serverId: 'remote-1',
        accessTokenExpiresAt: currentWindowExpiry
      })
    );

    await vi.advanceTimersByTimeAsync(24_000);

    expect(mockRenewServerAuthentication).toHaveBeenCalledOnce();
    expect(mockRenewServerAuthentication).toHaveBeenCalledWith('remote-1', true);
    client.dispose();
  });

  it('reschedules renewal when rotation reaches the current session window expiry', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    mockRenewServerAuthentication.mockResolvedValue('renewed-token');
    const currentWindowExpiry = 61_000;
    const client = new ServerConnection(
      makeConfig({
        token: 'my-token',
        serverId: 'remote-1',
        accessTokenExpiresAt: 31_000
      })
    );

    client.updateBearerSession('rotated-token', currentWindowExpiry);
    await vi.advanceTimersByTimeAsync(48_000);

    expect(mockRenewServerAuthentication).toHaveBeenCalledOnce();
    expect(mockRenewServerAuthentication).toHaveBeenCalledWith('remote-1', true);
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
    expect(mod.serverConnectionManager.originConnectBaseUrl).toBe(
      `${window.location.origin}/api/connect`
    );
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

  it('originClient ignores stored bearer credentials', async () => {
    const mod = await import('./serverConnection.svelte');
    mockServers.set('my-home', {
      id: 'my-home',
      url: window.location.origin,
      token: 'origin-token'
    });

    mod.serverConnectionManager.destroyClient('my-home');
    expect(mod.serverConnectionManager.originClient.bearerToken).toBeNull();
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
