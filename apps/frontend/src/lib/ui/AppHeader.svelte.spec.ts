import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppHeader from './AppHeader.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    servers: [] as Array<{ id: string }>,
    activeServer: '',
    activeStore: undefined as undefined,
    getStore: vi.fn(),
    pushState: vi.fn(),
    toggleSidebar: vi.fn(),
    openQuickSwitcher: vi.fn()
  }
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
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
    mocks.getStore.mockReset();
    mocks.pushState.mockReset();
  });

  it('hides notifications when no servers are registered', () => {
    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).toBeNull();
  });

  it('shows notifications when a server is registered', () => {
    mocks.servers = [{ id: 'remote' }];
    mocks.getStore.mockReturnValue({ notifications: { count: 0 } });

    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/notifications"]')).not.toBeNull();
  });

  it('opens the About Chatto dialog from the frontend version', () => {
    const { container } = render(AppHeader);

    (container.querySelector('button[aria-label="About Chatto"]') as HTMLButtonElement).click();

    expect(mocks.pushState).toHaveBeenCalledWith('', { modal: { type: 'aboutChatto' } });
  });
});
