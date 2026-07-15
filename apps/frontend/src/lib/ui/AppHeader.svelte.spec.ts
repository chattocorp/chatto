import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppHeader from './AppHeader.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    servers: [] as Array<{ id: string }>,
    getStore: vi.fn(),
    pushState: vi.fn(),
    toggleSidebar: vi.fn(),
    openQuickSwitcher: vi.fn()
  }
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState }));
vi.mock('$app/paths', () => ({ assets: '', base: '', resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: { url: new URL('https://chatto.test/') } }));
vi.mock('$app/environment', () => ({ version: '' }));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => '' }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    get originServer() {
      return undefined;
    },
    getStore: mocks.getStore,
    tryGetStore: () => undefined
  }
}));
vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    originClient: {
      showConnectionLostIcon: false,
      showConnectionLostBanner: false
    }
  }
}));
vi.mock('$lib/state/globals.svelte', () => ({
  sidebarNav: {
    isOpen: false,
    toggle: mocks.toggleSidebar
  },
  quickSwitcher: {
    open: mocks.openQuickSwitcher
  }
}));

describe('AppHeader', () => {
  beforeEach(() => {
    mocks.servers = [];
    mocks.getStore.mockReset();
  });

  it('hides notifications when no servers are registered', () => {
    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).toBeNull();
    expect(container.querySelector('a[href="/settings?returnTo=%2F"]')).not.toBeNull();
  });

  it('shows notifications when a server is registered', () => {
    mocks.servers = [{ id: 'remote' }];
    mocks.getStore.mockReturnValue({ notifications: { count: 0 } });

    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).not.toBeNull();
  });
});
