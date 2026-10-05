import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ReactiveMap, ReactiveSet } from '../reactivity/index.js';
import type { CurrentUser } from '../api/viewer.js';

type StoreMock = {
  currentUser: { user?: CurrentUser; loading: boolean };
  isAuthenticated: boolean;
  serverInfo: { isSupportedVersion: boolean };
  realtimeSync: { serverId: string };
  realtimeProjectionHandler?: () => void;
};

const mocks = vi.hoisted(() => ({
  originServerId: 'origin' as string | null,
  servers: [{ id: 'origin' }, { id: 'remote' }],
  stores: null as unknown as ReactiveMap<string, StoreMock>,
  startedBuses: null as unknown as ReactiveSet<string>,
  synchronizeAuthenticatedServers: vi.fn(),
  needsRecovery: vi.fn(() => false),
  onSessionTerminated: vi.fn<(id: string, handler: (reason: string) => void) => () => void>(() =>
    vi.fn()
  ),
  clearServerAuthentication: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  fixedTokens: new Set<string>(),
  getClient: vi.fn((serverId: string) => ({ serverId })),
  recoveryFails: false
}));

vi.mock('./serverRecovery.js', async (original) => {
  const actual = await original<typeof import('./serverRecovery.js')>();
  return {
    startServerRecovery: (...args: Parameters<typeof actual.startServerRecovery>) => {
      if (mocks.recoveryFails) throw new Error('recovery could not start');
      return actual.startServerRecovery(...args);
    }
  };
});

import {
  startClientRuntime as startRuntime,
  type ClientRuntime,
  type ClientRuntimeParts
} from './runtime.js';

/** The client parts that the tested runtime drives, backed by the mocks. */
const parts = {
  registry: {
    needsRecovery: mocks.needsRecovery,
    recoverServer: async () => {},
    get originServer() {
      return mocks.originServerId ? { id: mocks.originServerId } : undefined;
    },
    get servers() {
      return mocks.servers;
    },
    isOriginServer: (serverId: string) => serverId === mocks.originServerId,
    clearServerAuthentication: mocks.clearServerAuthentication,
    handleAuthenticationRequired: mocks.handleAuthenticationRequired,
    hasFixedToken: (serverId: string) => mocks.fixedTokens.has(serverId),
    getStore: (serverId: string) => mocks.stores.get(serverId),
    tryGetStore: (serverId: string) => mocks.stores.get(serverId)
  },
  connections: { getClient: mocks.getClient },
  realtime: {
    synchronizeAuthenticatedServers: mocks.synchronizeAuthenticatedServers,
    getBus: (serverId: string) =>
      mocks.startedBuses.has(serverId)
        ? {
            onSessionTerminated: (handler: (reason: string) => void) =>
              mocks.onSessionTerminated(serverId, handler)
          }
        : undefined
  }
} as unknown as ClientRuntimeParts;
const startClientRuntime = () => startRuntime(parts);

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
    serverInfo: { isSupportedVersion: true },
    realtimeSync: { serverId: `${serverId}-sync` },
    realtimeProjectionHandler: vi.fn(),
    ...overrides
  };
}

describe('startClientRuntime', () => {
  let runtime: ClientRuntime | undefined;

  afterEach(() => {
    runtime?.stop();
    runtime = undefined;
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.startedBuses = new ReactiveSet(['origin', 'remote']);
    mocks.originServerId = 'origin';
    mocks.servers = [{ id: 'origin' }, { id: 'remote' }];
    mocks.stores = new ReactiveMap([
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

  it('leaves no effects behind when recovery cannot start', async () => {
    mocks.recoveryFails = true;
    try {
      expect(() => startClientRuntime()).toThrow('recovery could not start');
    } finally {
      mocks.recoveryFails = false;
    }
    await Promise.resolve();
    expect(mocks.synchronizeAuthenticatedServers).not.toHaveBeenCalled();
    runtime = startClientRuntime();
  });

  it('uses the account owner without changing it on start or stop', async () => {
    const origin = mocks.stores.get('origin')!;
    origin.currentUser = { user: originUser, loading: false };
    runtime = startClientRuntime();

    expect(origin.currentUser.user).toMatchObject({
      id: 'origin-user',
      presenceStatus: PresenceStatus.AWAY
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

    runtime.stop();
    expect(origin.currentUser.user).toBe(originUser);
  });

  it('hydrates a restored remote-only session without an active server', async () => {
    mocks.originServerId = null;
    mocks.servers = [{ id: 'remote' }];
    mocks.stores.delete('origin');

    runtime = startClientRuntime();

    await vi.waitFor(() =>
      expect(mocks.synchronizeAuthenticatedServers.mock.calls[0]).toEqual([
        [expect.objectContaining({ serverId: 'remote', projectionSupported: true })],
        null
      ])
    );
  });

  it('clears a remote session when its server confirms termination', async () => {
    runtime = startClientRuntime();
    await vi.waitFor(() =>
      expect(mocks.onSessionTerminated).toHaveBeenCalledWith('remote', expect.any(Function))
    );
    const handler = mocks.onSessionTerminated.mock.calls.find(([id]) => id === 'remote')?.[1];
    handler?.('revoked');
    await vi.waitFor(() => expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('remote'));
  });

  it('ends a fixed-token session instead of clearing it when the server terminates it', async () => {
    mocks.fixedTokens.add('remote');
    runtime = startClientRuntime();
    await vi.waitFor(() =>
      expect(mocks.onSessionTerminated).toHaveBeenCalledWith('remote', expect.any(Function))
    );
    const handler = mocks.onSessionTerminated.mock.calls.find(([id]) => id === 'remote')?.[1];
    handler?.('revoked');
    await vi.waitFor(() =>
      expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote')
    );
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    mocks.fixedTokens.clear();
  });

  it('listens for remote session termination when the bus starts later', async () => {
    mocks.startedBuses.delete('remote');
    runtime = startClientRuntime();
    await Promise.resolve();
    expect(mocks.onSessionTerminated).not.toHaveBeenCalledWith('remote', expect.any(Function));

    mocks.startedBuses.add('remote');

    await vi.waitFor(() =>
      expect(mocks.onSessionTerminated).toHaveBeenCalledWith('remote', expect.any(Function))
    );
  });

  it('reconciles late session restoration and compatibility discovery', async () => {
    mocks.originServerId = null;
    mocks.servers = [{ id: 'remote' }];
    mocks.stores = new ReactiveMap([['remote', store('remote')]]);
    runtime = startClientRuntime();
    mocks.synchronizeAuthenticatedServers.mockClear();

    mocks.stores.set(
      'remote',
      store('remote', {
        currentUser: { user: { id: 'remote-user' } as CurrentUser, loading: false },
        isAuthenticated: true,
        serverInfo: { isSupportedVersion: false }
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
        serverInfo: { isSupportedVersion: true }
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

describe('startClientRuntime active server', () => {
  it('keeps the selected server live and coalesces synchronous changes', async () => {
    mocks.originServerId = null;
    mocks.servers = [{ id: 'remote' }];
    mocks.startedBuses = new ReactiveSet();
    mocks.stores = new ReactiveMap([
      ['remote', store('remote', { isAuthenticated: true, currentUser: { loading: false } })]
    ]);
    const runtime = startClientRuntime();
    await vi.waitFor(() => expect(mocks.synchronizeAuthenticatedServers).toHaveBeenCalled());
    mocks.synchronizeAuthenticatedServers.mockClear();

    runtime.setActiveServer('other');
    runtime.setActiveServer('remote');
    await Promise.resolve();

    expect(mocks.synchronizeAuthenticatedServers).toHaveBeenCalledExactlyOnceWith(
      [expect.objectContaining({ serverId: 'remote' })],
      'remote'
    );
    runtime.stop();
  });
});
