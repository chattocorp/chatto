import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { SvelteMap } from 'svelte/reactivity';
import type { CurrentUser } from '$lib/api-client/viewer';

type StoreMock = {
  currentUser: { user?: CurrentUser; loading: boolean };
  isAuthenticated: boolean;
  serverInfo: { supportsRealtimeProjection: boolean };
  realtimeSync: { serverId: string };
  realtimeProjectionHandler?: () => void;
};

const mocks = vi.hoisted(() => ({
  originServerId: 'origin' as string | null,
  activeServerId: '' as string,
  servers: [{ id: 'origin' }, { id: 'remote' }],
  stores: null as unknown as SvelteMap<string, StoreMock>,
  synchronizeAuthenticatedServers: vi.fn(),
  getClient: vi.fn((serverId: string) => ({ serverId }))
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServerId
}));

vi.mock('$app/state', () => ({
  page: { route: { id: '/login' } }
}));

vi.mock('./registry.svelte', () => ({
  serverRegistry: {
    get originServer() {
      return mocks.originServerId ? { id: mocks.originServerId } : undefined;
    },
    get servers() {
      return mocks.servers;
    },
    getStore: (serverId: string) => mocks.stores.get(serverId),
    tryGetStore: (serverId: string) => mocks.stores.get(serverId)
  }
}));

vi.mock('./serverConnection.svelte', () => ({
  serverConnectionManager: { getClient: mocks.getClient }
}));

vi.mock('./eventBus.svelte', () => ({
  eventBusManager: {
    synchronizeAuthenticatedServers: mocks.synchronizeAuthenticatedServers
  }
}));

import ServerRuntimeCoordinator from './ServerRuntimeCoordinator.svelte';

const originUser: CurrentUser = {
  id: 'origin-user',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  customStatus: null,
  presenceStatus: PresenceStatus.AWAY,
  hasVerifiedEmail: true,
  hasPassword: true,
  viewerCanDeleteAccount: true,
  lastLoginChange: null,
  settings: null
};

function store(serverId: string, overrides: Partial<StoreMock> = {}): StoreMock {
  return {
    currentUser: { loading: false },
    isAuthenticated: false,
    serverInfo: { supportsRealtimeProjection: true },
    realtimeSync: { serverId: `${serverId}-sync` },
    realtimeProjectionHandler: vi.fn(),
    ...overrides
  };
}

describe('ServerRuntimeCoordinator', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.originServerId = 'origin';
    mocks.activeServerId = '';
    mocks.servers = [{ id: 'origin' }, { id: 'remote' }];
    mocks.stores = new SvelteMap([
      ['origin', store('origin', { currentUser: { loading: true } })],
      [
        'remote',
        store('remote', {
          currentUser: { user: { id: 'remote-user' } as CurrentUser, loading: false },
          isAuthenticated: true
        })
      ]
    ]);
    Object.defineProperty(mocks.stores.get('origin')!, 'isAuthenticated', {
      get() {
        return Boolean(this.currentUser.user);
      }
    });
  });

  it('installs the origin viewer before the initial transport reconciliation', async () => {
    const { unmount } = render(ServerRuntimeCoordinator, { props: { user: originUser } });
    const origin = mocks.stores.get('origin')!;

    expect(origin.currentUser.user).toMatchObject({
      id: 'origin-user',
      presenceStatus: PresenceStatus.ONLINE
    });
    expect(origin.currentUser.loading).toBe(false);
    await vi.waitFor(() =>
      expect(mocks.synchronizeAuthenticatedServers.mock.calls[0]).toEqual([
        [
          expect.objectContaining({ serverId: 'origin' }),
          expect.objectContaining({ serverId: 'remote' })
        ],
        null
      ])
    );

    unmount();
    expect(origin.currentUser.user).toBeUndefined();
  });

  it('hydrates a restored remote-only session without an active chat route', async () => {
    mocks.originServerId = null;
    mocks.servers = [{ id: 'remote' }];
    mocks.stores.delete('origin');

    render(ServerRuntimeCoordinator, { props: { user: null } });

    await vi.waitFor(() =>
      expect(mocks.synchronizeAuthenticatedServers.mock.calls[0]).toEqual([
        [expect.objectContaining({ serverId: 'remote', projectionSupported: true })],
        null
      ])
    );
  });

  it('reconciles late session restoration and compatibility discovery', async () => {
    mocks.originServerId = null;
    mocks.servers = [{ id: 'remote' }];
    mocks.stores = new SvelteMap([['remote', store('remote')]]);
    render(ServerRuntimeCoordinator, { props: { user: null } });
    mocks.synchronizeAuthenticatedServers.mockClear();

    mocks.stores.set(
      'remote',
      store('remote', {
        currentUser: { user: { id: 'remote-user' } as CurrentUser, loading: false },
        isAuthenticated: true,
        serverInfo: { supportsRealtimeProjection: false }
      })
    );
    await vi.waitFor(() =>
      expect(mocks.synchronizeAuthenticatedServers).toHaveBeenCalledWith(
        [expect.objectContaining({ serverId: 'remote', projectionSupported: false })],
        null
      )
    );

    mocks.synchronizeAuthenticatedServers.mockClear();
    mocks.stores.set(
      'remote',
      store('remote', {
        currentUser: { user: { id: 'remote-user' } as CurrentUser, loading: false },
        isAuthenticated: true,
        serverInfo: { supportsRealtimeProjection: true }
      })
    );
    await vi.waitFor(() =>
      expect(mocks.synchronizeAuthenticatedServers).toHaveBeenCalledWith(
        [expect.objectContaining({ serverId: 'remote', projectionSupported: true })],
        null
      )
    );
  });
});
