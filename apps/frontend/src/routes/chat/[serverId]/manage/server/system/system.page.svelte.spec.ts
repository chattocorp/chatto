import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import SystemPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  getAdminSystemInfo: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {},
    connection: {
      queryScope: 'system-test',
      apiConfig: {
        baseUrl: '/api/connect',
        bearerToken: 'token'
      }
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/api-client/adminDiagnostics', async () => {
  const actual = await vi.importActual<typeof import('$lib/api-client/adminDiagnostics')>(
    '$lib/api-client/adminDiagnostics'
  );
  return {
    ...actual,
    getAdminSystemInfo: mocks.getAdminSystemInfo
  };
});

const systemInfo = {
  connection: {
    connected: true,
    serverId: 'nats-1',
    serverName: 'test-server',
    version: '2.11.0',
    maxPayload: 1024,
    rtt: '1ms'
  },
  account: {
    memory: 1000,
    memoryUsed: 100,
    storage: 2000,
    storageUsed: 200,
    streams: 10,
    streamsUsed: 2,
    consumers: 20,
    consumersUsed: 3
  },
  accountAvailable: true,
  nats: {
    totalMessages: 10,
    totalBytes: 1000,
    totalConsumerPending: 0,
    totalAckPending: 0,
    streams: [],
    consumers: []
  },
  natsAvailable: true,
  stats: {
    userCount: 4,
    channelRoomCount: 2,
    dmRoomCount: 1
  },
  statsAvailable: true,
  projections: [],
  projectionsAvailable: true,
  assetCleanup: {
    available: false,
    health: 'unavailable',
    pendingCount: 0,
    oldestPendingAt: null,
    passInProgress: false,
    lastPassAt: null,
    lastSuccessfulPassAt: null,
    updatedAt: null,
    lastPassFailed: false,
    lastInspectedSequence: '0',
    latestDeletionSequence: '0'
  },
  durableWorkers: []
};

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe('server admin system diagnostics', () => {
  beforeEach(() => {
    queryClient.clear();
    mocks.getAdminSystemInfo.mockReset();
    mocks.getAdminSystemInfo.mockResolvedValue(systemInfo);
  });

  afterEach(() => queryClient.clear());

  it('passes query cancellation through and reuses a fresh cached snapshot', async () => {
    const first = render(SystemPage);
    await settle();

    expect(mocks.getAdminSystemInfo).toHaveBeenCalledWith(
      { baseUrl: '/api/connect', bearerToken: 'token' },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(first.container.textContent).toContain('test-server');

    first.unmount();
    const second = render(SystemPage);
    await settle();

    expect(second.container.textContent).toContain('test-server');
    expect(mocks.getAdminSystemInfo).toHaveBeenCalledOnce();
  });

  it('clears and refetches a still-authorized mounted snapshot after an admin cache purge', async () => {
    const refreshed = deferred<typeof systemInfo>();
    mocks.getAdminSystemInfo
      .mockResolvedValueOnce(systemInfo)
      .mockReturnValueOnce(refreshed.promise);
    const { container } = render(SystemPage);
    await settle();
    expect(container.textContent).toContain('test-server');

    removeRegisteredAdminQueries('origin');
    await vi.waitFor(() => expect(mocks.getAdminSystemInfo).toHaveBeenCalledTimes(2));
    expect(container.textContent).not.toContain('test-server');

    refreshed.resolve({
      ...systemInfo,
      connection: { ...systemInfo.connection, serverName: 'refreshed-server' }
    });
    await vi.waitFor(() => expect(container.textContent).toContain('refreshed-server'));
  });

  it('keeps unrelated diagnostics visible when JetStream telemetry is unavailable', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({ ...systemInfo, natsAvailable: false });
    const { container } = render(SystemPage);
    await settle();

    expect(container.textContent).toContain('test-server');
    expect(container.textContent).toContain('Unavailable');
    expect(container.textContent).toContain('Projection Summary');
    expect(container.textContent).not.toContain('Stream Activity');
  });
});
