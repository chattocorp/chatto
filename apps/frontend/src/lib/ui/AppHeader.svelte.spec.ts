import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppHeader from './AppHeader.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    servers: [] as Array<{ id: string }>,
    activeServer: '',
    activeStore: undefined as undefined,
    authenticated: {} as Record<string, boolean>,
    getStore: vi.fn(),
    pushState: vi.fn(),
    toggleSidebar: vi.fn(),
    openQuickSwitcher: vi.fn()
  }
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    params?.serverId ? path.replace('[serverId]', params.serverId) : path
}));
vi.mock('$app/environment', () => ({ version: '0.5.0-test' }));
vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServer
}));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    get originServer() {
      return undefined;
    },
    isAuthenticated: (id: string) => mocks.authenticated[id] === true,
    firstAuthenticatedServerId: () =>
      mocks.servers.find((server) => mocks.authenticated[server.id])?.id,
    isOriginServer: () => false,
    getServer: (id: string) =>
      mocks.servers.find((server) => server.id === id)
        ? { id, url: `https://${id}.example.com` }
        : undefined,
    getStore: mocks.getStore,
    tryGetStore: (id: string) => (id === mocks.activeServer ? mocks.activeStore : undefined)
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
    mocks.activeServer = '';
    mocks.activeStore = undefined;
    mocks.authenticated = {};
    mocks.getStore.mockReset();
    mocks.pushState.mockReset();
  });

  it('hides notifications when no servers are registered', () => {
    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).toBeNull();
    expect(container.querySelector('a[href="/chat/preferences"]')).not.toBeNull();
  });

  it('shows notifications when a server is registered', () => {
    mocks.servers = [{ id: 'remote' }];
    mocks.getStore.mockReturnValue({ notifications: { count: 0 } });

    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).not.toBeNull();
    expect(container.querySelector('a[href="/chat/preferences"]')).not.toBeNull();
  });

  it('treats a server without a store as having no unread notifications', () => {
    mocks.servers = [{ id: 'remote' }];

    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="notifications-unread-dot"]')).toBeNull();
  });

  it('opens the canonical Settings entry point for the active authenticated server', () => {
    mocks.servers = [{ id: 'remote' }];
    mocks.activeServer = 'remote';
    mocks.authenticated = { remote: true };
    mocks.getStore.mockReturnValue({ notifications: { count: 0 } });

    const { container } = render(AppHeader);

    expect(
      container.querySelector('a[href="/chat/remote.example.com/settings"]')
    ).not.toBeNull();
    expect(container.querySelector('a[href="/chat/preferences"]')).toBeNull();
  });

  it('opens the About Chatto dialog from the frontend version', () => {
    const { container } = render(AppHeader);

    (container.querySelector('button[aria-label="About Chatto"]') as HTMLButtonElement).click();

    expect(mocks.pushState).toHaveBeenCalledWith('', { modal: { type: 'aboutChatto' } });
  });
});
