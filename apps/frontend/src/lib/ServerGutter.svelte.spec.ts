import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { SvelteMap } from 'svelte/reactivity';

type ServerMock = {
  id: string;
  reauthRequiredAt: number | null;
};

type StoreMock = {
  isAuthenticated: boolean;
  currentUser: { user?: { id: string } };
};

const mocks = vi.hoisted(() => ({
  servers: [] as ServerMock[],
  stores: new Map<string, StoreMock>()
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    tryGetStore: (serverId: string) => mocks.stores.get(serverId)
  }
}));

vi.mock('$lib/accountData/sync.svelte', () => ({
  accountDataSync: {
    status: 'disconnected',
    providerLabel: null,
    initialize: vi.fn(() => Promise.resolve()),
    connect: vi.fn(() => Promise.resolve())
  }
}));

vi.mock('$lib/clientConfig', () => ({
  getClientConfiguration: vi.fn(async () => ({ version: 1, authling: null }))
}));

vi.mock('./ServerSidebarEntry.svelte', async () => ({
  default: (await import('./ServerGutterEntryMock.svelte')).default
}));

vi.mock('$lib/ui/ScrollFader.svelte', async () => ({
  default: (await import('./ServerGutterScrollFaderMock.svelte')).default
}));

import ServerGutter from './ServerGutter.svelte';

beforeEach(() => {
  mocks.servers = [];
  mocks.stores = new SvelteMap();
});

describe('ServerGutter', () => {
  it('keeps an unauthenticated registered server available for navigation', () => {
    mocks.servers = [{ id: 'origin', reauthRequiredAt: null }];
    mocks.stores.set('origin', {
      isAuthenticated: false,
      currentUser: {}
    });

    const { container } = render(ServerGutter);

    expect(container.querySelector('[data-testid="server-entry"]')?.textContent).toBe('origin');
  });

  it('does not render a server until its state store exists', () => {
    mocks.servers = [{ id: 'origin', reauthRequiredAt: null }];

    const { container } = render(ServerGutter);

    expect(container.querySelector('[data-testid="server-entry"]')).toBeNull();
  });

  it('remounts an entry when authentication replaces its same-ID store', async () => {
    mocks.servers = [{ id: 'remote', reauthRequiredAt: null }];
    mocks.stores.set('remote', {
      isAuthenticated: true,
      currentUser: { user: { id: 'user-1' } }
    });
    const { container } = render(ServerGutter);
    const originalEntry = container.querySelector('[data-testid="server-entry"]');

    mocks.stores.set('remote', {
      isAuthenticated: true,
      currentUser: { user: { id: 'user-1' } }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="server-entry"]')).not.toBe(originalEntry);
    });
  });
});
