import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import SystemTestHarness from './SystemTestHarness.svelte';
import type { SystemSection } from './systemSections';

const mocks = vi.hoisted(() => ({
  getAdminSystemInfo: vi.fn()
}));

let routeId = '/chat/[serverId]/manage/server/system';

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { serverId: 'origin' };
    },
    get route() {
      return { id: routeId };
    }
  }
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

let server: TestServerScope;

vi.mock('$lib/api/adminDiagnostics', async () => {
  const actual = await vi.importActual<typeof import('$lib/api/adminDiagnostics')>(
    '$lib/api/adminDiagnostics'
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
  durableWorkers: [],
  livekit: {
    connectionState: 'not_configured',
    connectionError: '',
    enabled: false,
    configured: false,
    url: '',
    apiKey: '',
    separateWebhookKey: false,
    webhookUrl: '',
    insecureUrl: false,
    lastWebhookAt: null as Date | null,
    lastRejectedWebhookAt: null as Date | null
  }
};

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

const routeIds: Record<SystemSection, string> = {
  overview: '/chat/[serverId]/manage/server/system',
  streams: '/chat/[serverId]/manage/server/system/streams',
  projections: '/chat/[serverId]/manage/server/system/projections',
  workers: '/chat/[serverId]/manage/server/system/workers',
  livekit: '/chat/[serverId]/manage/server/system/livekit'
};

function renderSection(section: SystemSection = 'overview') {
  routeId = routeIds[section];
  return render(SystemTestHarness, { props: { section } });
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
    server = createTestServerScope({ serverId: 'origin' });
    mocks.getAdminSystemInfo.mockReset();
    mocks.getAdminSystemInfo.mockResolvedValue(systemInfo);
  });

  afterEach(() => queryClient.clear());

  it('passes query cancellation through and reuses a fresh cached snapshot', async () => {
    const first = renderSection();
    await settle();

    expect(mocks.getAdminSystemInfo).toHaveBeenCalledWith(
      server.scope.connection.apiConfig,
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(first.container.textContent).toContain('test-server');

    first.unmount();
    const second = renderSection();
    await settle();

    expect(second.container.textContent).toContain('test-server');
    expect(mocks.getAdminSystemInfo).toHaveBeenCalledOnce();
  });

  it('clears and refetches a still-authorized mounted snapshot after an admin cache purge', async () => {
    const refreshed = deferred<typeof systemInfo>();
    mocks.getAdminSystemInfo
      .mockResolvedValueOnce(systemInfo)
      .mockReturnValueOnce(refreshed.promise);
    const { container } = renderSection();
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

  it('summarizes health, usage, and server counts at the top', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({
      ...systemInfo,
      account: { ...systemInfo.account, storageUsed: 1600 }
    });
    const { container } = renderSection();
    await settle();

    expect(container.textContent).toContain('Needs attention');
    expect(container.textContent).toContain('80% of the nearest limit used');
    expect(container.textContent).toMatch(/Users\s*4/);
    expect(container.textContent).toContain('1 direct message');
    expect(container.querySelector('[role="meter"][aria-label="File Storage"]')).not.toBeNull();
  });

  it('marks critical checks with an accessible status', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({
      ...systemInfo,
      connection: { ...systemInfo.connection, connected: false }
    });
    const { container } = renderSection();
    await settle();

    expect(container.textContent).toContain('Problems found');
    expect(container.textContent).toContain('Not connected to the NATS broker');
    expect(
      container.querySelector('[data-health="critical"] [role="img"]')?.getAttribute('aria-label')
    ).toBe('Problem');
  });

  it('reads a new snapshot when the refresh action is used', async () => {
    const screen = renderSection();
    await settle();
    expect(mocks.getAdminSystemInfo).toHaveBeenCalledOnce();

    await screen.getByRole('button', { name: 'Refresh' }).click();
    await vi.waitFor(() => expect(mocks.getAdminSystemInfo).toHaveBeenCalledTimes(2));
  });

  it('keeps unrelated diagnostics visible when JetStream telemetry is unavailable', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({ ...systemInfo, natsAvailable: false });
    const overview = renderSection();
    await settle();

    expect(overview.container.textContent).toContain('test-server');
    expect(overview.container.textContent).toContain(
      'No problems found. Some checks have no data.'
    );
    expect(overview.container.textContent).not.toContain('Stored Data');
    overview.unmount();

    const streams = renderSection('streams');
    await settle();
    expect(streams.container.textContent).toContain(
      'Stream and consumer details are not available.'
    );
    streams.unmount();

    const projections = renderSection('projections');
    await settle();
    expect(projections.container.textContent).toContain('Projection Summary');
  });

  it('shows one tab per section and marks the current one', async () => {
    const screen = renderSection('workers');
    await settle();

    const nav = screen.getByRole('navigation', { name: 'System sections' });
    for (const name of ['Overview', 'Streams', 'Projections', 'Workers', 'LiveKit']) {
      await expect.element(nav.getByRole('link', { name, exact: true })).toBeVisible();
    }
    await expect
      .element(nav.getByRole('link', { name: 'Projections', exact: true }))
      .toHaveAttribute('href', '/chat/-/manage/server/system/projections');
    await expect
      .element(nav.getByRole('link', { name: 'Workers', exact: true }))
      .toHaveAttribute('aria-current', 'page');
    expect(screen.container.textContent).toContain('Background Workers');
  });

  it('hides the LiveKit tab when the server does not report LiveKit status', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({
      ...systemInfo,
      livekit: { ...systemInfo.livekit, connectionState: 'unavailable' }
    });
    const screen = renderSection();
    await settle();

    const nav = screen.getByRole('navigation', { name: 'System sections' });
    await expect.element(nav.getByRole('link', { name: 'Workers', exact: true })).toBeVisible();
    expect(nav.getByRole('link', { name: 'LiveKit' }).query()).toBeNull();
  });

  it('marks the tab of a failing section and links the check to it', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({
      ...systemInfo,
      projections: [
        {
          key: 'rooms',
          name: 'rooms',
          subjects: [],
          streamLastSequence: '9',
          metrics: [],
          started: true,
          failed: true,
          failure: 'boom',
          failedSequence: '7',
          lastAppliedSequence: '6',
          matchingStreamSequence: '9',
          lag: 3,
          entryCount: 1,
          estimatedBytes: 10,
          averageEntryBytes: 10,
          startupDurationSeconds: 0.5
        }
      ]
    });
    const screen = renderSection();
    await settle();

    const nav = screen.getByRole('navigation', { name: 'System sections' });
    await expect.element(nav.getByRole('link', { name: 'Projections (Problem)' })).toBeVisible();
    expect(screen.container.querySelectorAll('[data-tab-status]')).toHaveLength(1);

    const health = screen.container.querySelector('[data-health="critical"]');
    expect(health?.querySelector('a')?.getAttribute('href')).toBe(
      '/chat/-/manage/server/system/projections'
    );
  });

  it('shows that LiveKit is not configured without setup details', async () => {
    const overview = renderSection();
    await settle();

    expect(overview.container.querySelectorAll('[data-health]')).toHaveLength(5);
    expect(overview.container.textContent).toContain('Everything is running normally');
    overview.unmount();

    const { container } = renderSection('livekit');
    await settle();
    const panel = container.querySelector('[data-testid="livekit-panel"]');
    expect(panel?.textContent).toContain('Not configured');
    expect(panel?.textContent).not.toContain('Webhook URL');
  });

  it('reports a LiveKit setup problem in the health summary and the LiveKit panel', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({
      ...systemInfo,
      livekit: {
        ...systemInfo.livekit,
        connectionState: 'unauthorized',
        connectionError: 'twirp error unauthenticated: invalid token',
        enabled: true,
        configured: true,
        url: 'wss://livekit.example',
        apiKey: 'APIkey123',
        webhookUrl: 'https://chat.example/webhooks/livekit'
      }
    });
    const overview = renderSection();
    await settle();

    expect(overview.container.textContent).toContain('Problems found');
    expect(overview.container.textContent).toContain('LiveKit rejected the API key');
    overview.unmount();

    const { container } = renderSection('livekit');
    await settle();
    const panel = container.querySelector('[data-testid="livekit-panel"]');
    expect(panel?.textContent).toContain('Credentials rejected');
    expect(panel?.textContent).toContain('twirp error unauthenticated: invalid token');
    expect(panel?.textContent).toContain('https://chat.example/webhooks/livekit');
    expect(panel?.textContent).toContain('APIkey123');
    expect(panel?.textContent).toContain('has not received a webhook from LiveKit');
  });

  it('warns about a rejected LiveKit webhook signature', async () => {
    mocks.getAdminSystemInfo.mockResolvedValue({
      ...systemInfo,
      livekit: {
        ...systemInfo.livekit,
        connectionState: 'ok',
        enabled: true,
        configured: true,
        url: 'wss://livekit.example',
        lastWebhookAt: new Date('2026-07-10T11:00:00Z'),
        lastRejectedWebhookAt: new Date('2026-07-10T12:00:00Z')
      }
    });
    const overview = renderSection();
    await settle();

    expect(overview.container.textContent).toContain('Needs attention');
    expect(overview.container.textContent).toContain('LiveKit webhooks rejected');
    overview.unmount();

    const { container } = renderSection('livekit');
    await settle();
    expect(container.textContent).toContain('signature that is not valid');
  });
});
