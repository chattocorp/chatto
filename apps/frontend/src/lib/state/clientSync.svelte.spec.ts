import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ClientSyncPendingTestHarness from './ClientSyncPendingTestHarness.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    apiByServer: new Map<string, ReturnType<typeof makeAPI>>(),
    servers: [] as Array<Record<string, unknown>>,
    homeServerId: 'a' as string | null,
    capable: new Set<string>(),
    authenticated: new Set<string>(),
    setLocale: vi.fn()
  }
}));

function makeAPI() {
  return {
    getPreferences: vi.fn(),
    updatePreferences: vi.fn(),
    listKnownServers: vi.fn(),
    createKnownServer: vi.fn(),
    updateKnownServer: vi.fn(),
    deleteKnownServer: vi.fn(),
    setHomeServer: vi.fn()
  };
}

vi.mock('$lib/api-client/clientSync', () => ({
  ClientSyncTimeFormat: {
    TIME_FORMAT_UNSPECIFIED: 0,
    TIME_FORMAT_12_HOUR: 1,
    TIME_FORMAT_24_HOUR: 2
  },
  createClientSyncAPI: ({ serverId }: { serverId: string }) => mocks.apiByServer.get(serverId)
}));

vi.mock('$lib/i18n/runtime', () => ({
  getLocale: () => 'en-GB',
  setLocale: mocks.setLocale
}));

vi.mock('$lib/i18n/locales', () => ({
  selectableLocales: ['en-GB', 'de-DE']
}));

vi.mock('./server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: (serverId: string) => ({
      connectBaseUrl: `https://${serverId}.example/api/connect`,
      bearerToken: `${serverId}-token`
    })
  }
}));

vi.mock('./server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    get homeServerId() {
      return mocks.homeServerId;
    },
    getServer: (id: string) => mocks.servers.find((server) => server.id === id),
    tryGetStore: (id: string) =>
      mocks.servers.some((server) => server.id === id)
        ? {
            isAuthenticated: mocks.authenticated.has(id),
            currentUser: { user: { id: `${id}-user`, settings: undefined } }
          }
        : undefined,
    isAuthenticated: (id: string) => mocks.authenticated.has(id),
    isClientSyncCapable: (id: string) => mocks.capable.has(id),
    setHomeServer: (id: string) => {
      if (!mocks.servers.some((server) => server.id === id)) return false;
      mocks.homeServerId = id;
      return true;
    },
    addServer: (server: Record<string, unknown>) => mocks.servers.push(server),
    updateServer: (id: string, update: Record<string, unknown>) => {
      const server = mocks.servers.find((candidate) => candidate.id === id);
      if (server) Object.assign(server, update);
      return !!server;
    },
    removeServer: vi.fn(() => true)
  }
}));

import { ClientSyncState } from './clientSync.svelte';
import { TimeFormat } from '$lib/render/types';

function server(id: string) {
  return {
    id,
    url: `https://${id}.example`,
    name: id.toUpperCase(),
    iconUrl: null,
    token: `${id}-token`,
    userId: `${id}-user`,
    userLogin: id,
    userDisplayName: id.toUpperCase(),
    userAvatarUrl: null,
    reauthRequiredAt: null,
    addedAt: 1
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((fulfil) => (resolve = fulfil));
  return { promise, resolve };
}

function installAPI(serverId: string) {
  const api = makeAPI();
  api.getPreferences.mockResolvedValue({});
  api.updatePreferences.mockResolvedValue({});
  api.listKnownServers.mockResolvedValue({
    servers: [{ id: serverId, url: `https://${serverId}.example`, name: serverId.toUpperCase() }],
    homeServerId: serverId
  });
  api.createKnownServer.mockResolvedValue({});
  api.updateKnownServer.mockResolvedValue({});
  api.deleteKnownServer.mockResolvedValue(undefined);
  api.setHomeServer.mockResolvedValue({});
  mocks.apiByServer.set(serverId, api);
  return api;
}

describe('ClientSyncState', () => {
  beforeEach(() => {
    localStorage.clear();
    mocks.apiByServer.clear();
    mocks.servers = [server('a')];
    mocks.homeServerId = 'a';
    mocks.capable = new Set(['a']);
    mocks.authenticated = new Set(['a']);
    mocks.setLocale.mockReset();
    mocks.setLocale.mockResolvedValue(undefined);
  });

  it('preserves and uploads locale edits made while the initial load is pending', async () => {
    const api = installAPI('a');
    const remotePreferences = deferred<{ locale: string }>();
    api.getPreferences.mockReturnValue(remotePreferences.promise);
    const state = new ClientSyncState();

    const loading = state.load('a');
    await state.setLocale('de-DE');
    remotePreferences.resolve({ locale: 'en-GB' });
    await loading;

    expect(state.locale).toBe('de-DE');
    expect(api.updatePreferences).toHaveBeenCalledWith(
      expect.objectContaining({ locale: 'de-DE' }),
      ['locale', 'timezone', 'time_format']
    );
  });

  it('preserves and uploads display edits made while the initial load is pending', async () => {
    const api = installAPI('a');
    const remotePreferences = deferred<{ timezone: string }>();
    api.getPreferences.mockReturnValue(remotePreferences.promise);
    const state = new ClientSyncState();

    const loading = state.load('a');
    await state.setDisplaySettings('Europe/Berlin', TimeFormat.TwentyFourHour);
    remotePreferences.resolve({ timezone: 'America/New_York' });
    await loading;

    expect(state.timezone).toBe('Europe/Berlin');
    expect(state.timeFormat).toBe(TimeFormat.TwentyFourHour);
    expect(api.updatePreferences).toHaveBeenCalledWith(
      expect.objectContaining({ timezone: 'Europe/Berlin', timeFormat: 2 }),
      ['locale', 'timezone', 'time_format']
    );
  });

  it('keeps a remote home redirect pending until the destination is authenticated', async () => {
    const api = installAPI('a');
    api.getPreferences.mockResolvedValue({ locale: 'en-GB' });
    api.listKnownServers.mockResolvedValue({
      servers: [
        { id: 'a', url: 'https://a.example', name: 'A' },
        { id: 'b', url: 'https://b.example', name: 'B' }
      ],
      homeServerId: 'b'
    });
    const state = new ClientSyncState();

    await state.load('a');
    expect(mocks.homeServerId).toBe('a');
    expect(state.status).toBe('unavailable');
    expect(api.setHomeServer).not.toHaveBeenCalled();

    const restored = mocks.servers.find((server) => server.url === 'https://b.example');
    expect(restored).toBeDefined();
    const restoredId = String(restored?.id);
    mocks.authenticated.add(restoredId);
    mocks.capable.add(restoredId);
    state.tryFollowPendingRemoteHome();
    expect(mocks.homeServerId).toBe(restoredId);
  });

  it('reactively reveals pending-home recovery after an asynchronous load', async () => {
    const api = installAPI('a');
    const remoteDirectory = deferred<{
      servers: Array<{ id: string; url: string; name: string }>;
      homeServerId: string;
    }>();
    api.listKnownServers.mockReturnValue(remoteDirectory.promise);
    const state = new ClientSyncState();
    const { container } = render(ClientSyncPendingTestHarness, { state });
    expect(container.querySelector('[data-testid="pending-home"]')).toBeNull();

    const loading = state.load('a');
    remoteDirectory.resolve({
      servers: [
        { id: 'a', url: 'https://a.example', name: 'A' },
        { id: 'b', url: 'https://b.example', name: 'B' }
      ],
      homeServerId: 'b'
    });
    await loading;

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="pending-home"]')).not.toBeNull();
    });
  });

  it('lets the user keep the current capable home when a remote destination is unusable', async () => {
    const api = installAPI('a');
    api.listKnownServers.mockResolvedValue({
      servers: [
        { id: 'a', url: 'https://a.example', name: 'A' },
        { id: 'b', url: 'https://b.example', name: 'B' }
      ],
      homeServerId: 'b'
    });
    const state = new ClientSyncState();

    await state.load('a');
    expect(state.hasPendingRemoteHome).toBe(true);
    await expect(state.selectHomeServer('a')).resolves.toBe(true);

    expect(state.hasPendingRemoteHome).toBe(false);
    expect(state.status).toBe('synced');
    expect(api.setHomeServer).toHaveBeenCalledWith('a');
  });

  it('does not follow another account into a pending remote home', async () => {
    const api = installAPI('a');
    api.listKnownServers.mockResolvedValue({
      servers: [
        { id: 'a', url: 'https://a.example', name: 'A' },
        { id: 'b', url: 'https://b.example', name: 'B' }
      ],
      homeServerId: 'b'
    });
    const state = new ClientSyncState();

    await state.load('a');
    const restored = mocks.servers.find((server) => server.url === 'https://b.example');
    const restoredId = String(restored?.id);
    mocks.authenticated.delete('a');
    mocks.authenticated.add(restoredId);
    mocks.capable.add(restoredId);
    state.tryFollowPendingRemoteHome();

    expect(mocks.homeServerId).toBe('a');
    expect(state.hasPendingRemoteHome).toBe(false);
  });

  it('rolls back a home move when the destination transfer fails', async () => {
    mocks.servers.push(server('b'));
    mocks.authenticated.add('b');
    mocks.capable.add('b');
    installAPI('a');
    const destination = installAPI('b');
    destination.updatePreferences.mockRejectedValue(new Error('offline'));
    const state = new ClientSyncState();

    await expect(state.selectHomeServer('b')).resolves.toBe(false);
    expect(mocks.homeServerId).toBe('a');
  });
});
