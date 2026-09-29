import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { formatPageTitle } from './pageTitle';

let current = $state<{ title: string; scope?: 'route' | 'app' } | null>(null);

const mocks = vi.hoisted(() => ({
  page: { params: {} as { serverId?: string }, route: { id: '/login' } },
  originServer: undefined as { id: string } | undefined,
  servers: [] as Array<{ id: string; url: string }>,
  stores: new Map<
    string,
    {
      isAuthenticated: boolean;
      serverInfo: { name: string };
      attention: {
        counts: {
          unreadNotificationCount: number;
          importantUnreadNotificationCount: number;
        };
      };
    }
  >()
}));

// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get originServer() {
      return mocks.originServer;
    },
    get servers() {
      return mocks.servers;
    },
    tryGetStore: (id: string) => mocks.stores.get(id)
  }
}));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return mocks.page.params;
    },
    get route() {
      return mocks.page.route;
    }
  }
}));

function store(name: string, importantUnreadNotificationCount = 0, isAuthenticated = true) {
  const result = $state({
    isAuthenticated,
    serverInfo: { name },
    attention: {
      counts: {
        unreadNotificationCount: importantUnreadNotificationCount + 4,
        importantUnreadNotificationCount
      }
    }
  });
  return result;
}

function setServers(
  entries: Array<{
    id: string;
    name: string;
    count?: number;
    isAuthenticated?: boolean;
    origin?: boolean;
  }>
) {
  mocks.servers = entries.map(({ id }) => ({ id, url: `https://${id}` }));
  mocks.stores.clear();
  mocks.originServer = undefined;

  for (const entry of entries) {
    mocks.stores.set(entry.id, store(entry.name, entry.count ?? 0, entry.isAuthenticated ?? true));
    if (entry.origin) mocks.originServer = { id: entry.id };
  }
}

function createTitleGetter() {
  let getTitle: (() => string) | undefined;
  const cleanup = $effect.root(() => {
    const title = $derived(formatPageTitle(current?.title, current?.scope));
    getTitle = () => title;
  });
  flushSync();
  if (!getTitle) throw new Error('Title getter was not initialized');
  return { getTitle, cleanup };
}

beforeEach(() => {
  const page = $state({ params: {}, route: { id: '/login' } });
  mocks.page = page;
  current = null;
  setServers([{ id: 'origin', name: 'Chatto Test', origin: true }]);
});

afterEach(() => {
  current = null;
});

describe('formatPageTitle', () => {
  it('falls back while registered server stores are unavailable', () => {
    setServers([
      { id: 'origin', name: 'Origin', origin: true, count: 5 },
      { id: 'remote', name: 'Remote', count: 2 }
    ]);
    mocks.stores.delete('origin');

    expect(formatPageTitle('Overview')).toBe('(2) Overview · Chatto');
    expect(formatPageTitle('Appearance', 'app')).toBe('(2) Appearance · Chatto');
  });

  it('uses the origin server name as the base title', () => {
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Chatto Test');

    cleanup();
  });

  it('combines the page title with the origin server name', () => {
    current = { title: 'Overview' };
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Overview · Chatto Test');

    cleanup();
  });

  it('falls back to Chatto without an origin server', () => {
    setServers([]);
    current = { title: 'Sign In' };
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Sign In · Chatto');

    cleanup();
  });

  it('prefixes only important authenticated unread notification counts across servers', () => {
    setServers([
      { id: 'origin', name: 'Chatto Test', count: 2, origin: true },
      { id: 'remote', name: 'Remote', count: 3 },
      { id: 'signed-out', name: 'Signed Out', count: 99, isAuthenticated: false }
    ]);
    current = { title: 'Overview' };
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('(5) Overview · Chatto Test');

    cleanup();
  });

  it('omits the count when only ambient notifications remain', () => {
    setServers([{ id: 'origin', name: 'Chatto Test', origin: true, count: 1 }]);
    const { getTitle, cleanup } = createTitleGetter();
    expect(getTitle()).toBe('(1) Chatto Test');

    mocks.stores.get('origin')!.attention.counts.importantUnreadNotificationCount = 0;
    flushSync();

    expect(getTitle()).toBe('Chatto Test');

    cleanup();
  });

  it('reacts when the page title segment changes', () => {
    current = { title: 'Overview' };
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Overview · Chatto Test');

    current.title = '#general';
    flushSync();
    expect(getTitle()).toBe('#general · Chatto Test');

    cleanup();
  });

  it('uses the route server and reacts to its name changing', () => {
    setServers([
      { id: 'origin', name: 'Origin', origin: true },
      { id: 'remote', name: 'Remote' }
    ]);
    mocks.page.params = { serverId: 'remote' };
    mocks.page.route = { id: '/chat/[serverId]/overview' };
    current = { title: 'Overview' };
    const { getTitle, cleanup } = createTitleGetter();
    expect(getTitle()).toBe('Overview · Remote');

    mocks.stores.get('remote')!.serverInfo.name = 'Renamed';
    flushSync();
    expect(getTitle()).toBe('Overview · Renamed');
    mocks.page.params = { serverId: '-' };
    flushSync();
    expect(getTitle()).toBe('Overview · Origin');
    mocks.page.params = {};
    mocks.page.route = { id: '/chat/servers' };
    flushSync();
    expect(getTitle()).toBe('Overview · Chatto');
    cleanup();
  });

  it.each(['missing', 'remote'])(
    'falls back to Chatto for unavailable server information: %s',
    (serverId) => {
      setServers([
        { id: 'origin', name: 'Origin', origin: true },
        { id: 'remote', name: '' }
      ]);
      mocks.page.params = { serverId };
      current = { title: 'Overview' };
      const { getTitle, cleanup } = createTitleGetter();
      expect(getTitle()).toBe('Overview · Chatto');
      cleanup();
    }
  );

  it('uses the origin server for the home route placeholder', () => {
    mocks.page.params = { serverId: '-' };
    current = { title: '#general' };
    const { getTitle, cleanup } = createTitleGetter();
    expect(getTitle()).toBe('#general · Chatto Test');
    cleanup();
  });

  it('uses product identity for app-wide chat routes', () => {
    mocks.page.route = { id: '/chat/notifications' };
    current = { title: 'Notifications' };
    const { getTitle, cleanup } = createTitleGetter();
    expect(getTitle()).toBe('Notifications · Chatto');
    cleanup();
  });

  it('uses product identity for app preferences within server routes and clears the override', () => {
    mocks.page.params = { serverId: '-' };
    current = { title: 'Appearance', scope: 'app' };
    const { getTitle, cleanup } = createTitleGetter();
    expect(getTitle()).toBe('Appearance · Chatto');
    current = null;
    flushSync();
    expect(getTitle()).toBe('Chatto Test');
    cleanup();
  });
});
