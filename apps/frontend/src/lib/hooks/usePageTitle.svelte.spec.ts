import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { titleState } from '$lib/state/globals.svelte';

const mocks = vi.hoisted(() => ({
  originServer: undefined as { id: string } | undefined,
  servers: [] as Array<{ id: string }>,
  stores: new Map<
    string,
    {
      isAuthenticated: boolean;
      serverInfo: { name: string };
      notifications: { count: number };
    }
  >()
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get originServer() {
      return mocks.originServer;
    },
    get servers() {
      return mocks.servers;
    },
    getStore: (id: string) => {
      const store = mocks.stores.get(id);
      if (!store) throw new Error(`No mocked store for ${id}`);
      return store;
    }
  }
}));

import { usePageTitle } from './usePageTitle.svelte';

function store(name: string, count = 0, isAuthenticated = true) {
  return {
    isAuthenticated,
    serverInfo: { name },
    notifications: { count }
  };
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
  mocks.servers = entries.map(({ id }) => ({ id }));
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
    getTitle = usePageTitle();
  });
  flushSync();
  if (!getTitle) throw new Error('Title getter was not initialized');
  return { getTitle, cleanup };
}

beforeEach(() => {
  titleState.clearPageTitle();
  setServers([{ id: 'origin', name: 'Chatto Test', origin: true }]);
});

afterEach(() => {
  titleState.clearPageTitle();
});

describe('usePageTitle', () => {
  it('uses the origin server name as the base title', () => {
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Chatto Test');

    cleanup();
  });

  it('combines the page title with the origin server name', () => {
    titleState.setPageTitle('Overview');
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Overview | Chatto Test');

    cleanup();
  });

  it('falls back to Chatto without an origin server', () => {
    setServers([]);
    titleState.setPageTitle('Sign In');
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('Sign In | Chatto');

    cleanup();
  });

  it('prefixes authenticated notification counts across servers', () => {
    setServers([
      { id: 'origin', name: 'Chatto Test', count: 2, origin: true },
      { id: 'remote', name: 'Remote', count: 3 },
      { id: 'signed-out', name: 'Signed Out', count: 99, isAuthenticated: false }
    ]);
    titleState.setPageTitle('Overview');
    const { getTitle, cleanup } = createTitleGetter();

    expect(getTitle()).toBe('(5) Overview | Chatto Test');

    cleanup();
  });

  it('reacts when the page title segment changes', () => {
    const { getTitle, cleanup } = createTitleGetter();

    titleState.setPageTitle('Overview');
    flushSync();
    expect(getTitle()).toBe('Overview | Chatto Test');

    titleState.setPageTitle('#general - Test Space');
    flushSync();
    expect(getTitle()).toBe('#general - Test Space | Chatto Test');

    cleanup();
  });
});
