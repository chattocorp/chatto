import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppHeader from './AppHeader.svelte';
import AppHeaderTestHarness from './AppHeaderTestHarness.svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import type { ServerStateStore } from '@chatto/client/server/store';
import { cdp, page } from 'vitest/browser';
import '../../app.css';
import { desktopUpdates } from '$lib/desktop/updates.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    servers: [] as Array<{ id: string }>,
    activeServer: '',
    callStore: null as ServerStateStore | null,
    activeStore: undefined as
      | {
          serverInfo: { motd: string };
          voiceCall: { connected: boolean };
          attention: {
            counts: {
              unreadNotificationCount: number;
              importantUnreadNotificationCount: number;
            };
          };
        }
      | undefined,
    authenticated: {} as Record<string, boolean>,
    getStore: vi.fn(),
    pushState: vi.fn(),
    toggleSidebar: vi.fn(),
    openQuickSwitcher: vi.fn()
  }
}));

// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    get originServer() {
      return undefined;
    },
    isAuthenticated: (id: string) => mocks.authenticated[id] === true,
    isOriginServer: () => false,
    getServer: (id: string) =>
      mocks.servers.find((server) => server.id === id)
        ? { id, url: `https://${id}.example.com` }
        : undefined,
    getStore: mocks.getStore,
    tryGetStore: (id: string) =>
      id === mocks.callStore?.serverId
        ? mocks.callStore
        : id === mocks.activeServer
          ? mocks.activeStore
          : undefined
  },
  serverConnectionManager: {
    originClient: {
      showConnectionLostIcon: false,
      showConnectionLostBanner: false
    }
  }
}));

vi.mock('$lib/serverCatalogue', () => ({
  firstAuthenticatedServerId: () =>
    mocks.servers.find((server) => mocks.authenticated[server.id])?.id
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState, goto: vi.fn() }));
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string, params?: Record<string, string>) =>
    params?.serverId ? path.replace('[serverId]', params.serverId) : path
}));
vi.mock('$app/environment', () => ({ version: '0.5.0-dev+f7b4e515c998' }));
vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServer
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
  it.each([320, 375, 420, 1280])(
    'keeps the other server call accessible at %i pixels',
    async (width) => {
      const leave = vi.fn();
      const call = createTestServerScope({
        serverId: 'call-server',
        store: {
          voiceCall: { connected: true, roomId: 'room-1', isMuted: true, leave },
          navigation: { rooms: [] },
          attention: { counts: { unreadNotificationCount: 0, importantUnreadNotificationCount: 0 } }
        }
      });
      mocks.activeServer = 'other-server';
      mocks.servers = [{ id: 'other-server' }, { id: 'call-server' }];
      mocks.callStore = call.scope.store;
      await page.viewport(width, 480);
      try {
        const { container, getByRole } = render(AppHeaderTestHarness);
        const header = container.querySelector('header')!;
        const bounds = header.getBoundingClientRect();
        const controls = header.querySelector('[data-testid="app-header-call-card"]')!;
        expect(controls).not.toBeNull();
        for (const button of controls.querySelectorAll('button')) {
          const rect = button.getBoundingClientRect();
          expect(rect.width).toBeGreaterThanOrEqual(28);
          expect(rect.height).toBeGreaterThanOrEqual(30);
          expect(rect.left).toBeGreaterThanOrEqual(bounds.left);
          expect(rect.right).toBeLessThanOrEqual(bounds.right);
          expect(rect.top).toBeGreaterThanOrEqual(bounds.top);
          expect(rect.bottom).toBeLessThanOrEqual(bounds.bottom);
        }
        expect(getComputedStyle(header.firstElementChild!).gap).toBe('12px');
        if (width >= 420) {
          expect(header.scrollWidth).toBeLessThanOrEqual(header.clientWidth);
        } else {
          // With Help beside About, narrow screens use the header's scroll fallback.
          // Preserve navigation spacing and the call controls' minimum target sizes.
          const about = header.querySelector<HTMLButtonElement>('button[title="About Chatto"]')!;
          const help = header.querySelector<HTMLAnchorElement>('a[href="/help"]')!;
          help.scrollIntoView({ block: 'nearest', inline: 'nearest' });
          expect(about.getBoundingClientRect().left).toBeGreaterThanOrEqual(bounds.left);
          expect(help.getBoundingClientRect().right).toBeLessThanOrEqual(bounds.right);
          expect(help.getBoundingClientRect().left).toBeGreaterThan(
            about.getBoundingClientRect().left
          );
          expect(about.getBoundingClientRect().right).toBeLessThanOrEqual(bounds.right);
        }
        await getByRole('button', { name: 'Leave call', exact: true }).click();
        expect(leave).toHaveBeenCalledOnce();
      } finally {
        mocks.callStore = null;
        await page.viewport(1280, 720);
      }
    }
  );
  it('shows an update-ready link only while a host update is ready', async () => {
    const { getByRole } = render(AppHeader);
    await expect.element(getByRole('link', { name: 'Update ready' })).not.toBeInTheDocument();
    desktopUpdates.status = {
      revision: 1,
      version: '0.5.0',
      status: 'ready',
      busy: false,
      progress: null,
      availableVersion: '0.6.0',
      automatic: true,
      channel: 'stable'
    };
    await expect
      .element(getByRole('link', { name: 'Update ready' }))
      .toHaveAttribute('href', '/chat/preferences/desktop');
    desktopUpdates.status = null;
    await expect.element(getByRole('link', { name: 'Update ready' })).not.toBeInTheDocument();
  });
  it('keeps the current server when opening cross-server notifications', () => {
    mocks.activeServer = 'remote';
    mocks.servers = [{ id: 'remote' }];
    mocks.authenticated = { remote: true };
    const { container } = render(AppHeader);
    expect(
      container.querySelector('a[href="/chat/remote.example.com/notifications"]')
    ).not.toBeNull();
    mocks.activeServer = '';
  });

  it('keeps global notifications available when the origin is signed out', () => {
    mocks.activeServer = 'origin';
    mocks.servers = [{ id: 'origin' }, { id: 'remote' }];
    mocks.authenticated = { remote: true };
    const { container } = render(AppHeader);
    expect(container.querySelector('a[href="/chat/notifications"]')).not.toBeNull();
  });
  it('hides on mobile with the keyboard and returns when it closes', async () => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
    await page.viewport(390, 800);
    const { getByRole, container } = render(AppHeader);
    const header = container.querySelector('header')!;
    const height = header.getBoundingClientRect().height;
    expect(height).toBeGreaterThan(0);
    try {
      document.body.setAttribute('data-keyboard-open', '');
      expect(header.getBoundingClientRect().height).toBe(0);
      await page.viewport(1024, 800);
      await expect.element(getByRole('banner')).toBeVisible();
      await page.viewport(390, 800);
      document.body.removeAttribute('data-keyboard-open');
      await expect.element(getByRole('banner')).toBeVisible();
      expect(header.getBoundingClientRect().height).toBe(height);
    } finally {
      await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
      document.body.removeAttribute('data-keyboard-open');
      await page.viewport(1280, 720);
    }
  });

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
    mocks.getStore.mockReturnValue({
      attention: { counts: { unreadNotificationCount: 0 } }
    });

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
    mocks.getStore.mockReturnValue({
      attention: { counts: { unreadNotificationCount: 0 } }
    });

    const { container } = render(AppHeader);

    expect(container.querySelector('a[href="/chat/remote.example.com/settings"]')).not.toBeNull();
    expect(container.querySelector('a[href="/chat/preferences"]')).toBeNull();
  });

  it('opens the About Chatto dialog from the info button', () => {
    const { container } = render(AppHeader);

    (container.querySelector('button[aria-label="About Chatto"]') as HTMLButtonElement).click();

    expect(mocks.pushState).toHaveBeenCalledWith('', { modal: { type: 'aboutChatto' } });
  });

  it.each([
    [390, false],
    [1280, true]
  ])('shows the client version at %i pixels: %s', async (width, visible) => {
    await page.viewport(width, 800);
    try {
      const { getByTestId } = render(AppHeader);
      const label = getByTestId('app-header-version');
      await expect.element(label).toHaveTextContent('v0.5.0-dev+f7b4e515c998');
      if (visible) await expect.element(label).toBeVisible();
      else await expect.element(label).not.toBeVisible();
    } finally {
      await page.viewport(1280, 720);
    }
  });

  it.each([320, 390, 1280])(
    'keeps the header height and truncates the MOTD at %i pixels',
    async (width) => {
      mocks.servers = [{ id: 'remote' }];
      mocks.activeServer = 'remote';
      const motd = '**Chatto HQ** · https://chatto.run\n\n' + 'Server news. '.repeat(30);
      await page.viewport(width, 800);
      try {
        const empty = render(AppHeader);
        const emptyHeight = empty.container.querySelector('header')!.getBoundingClientRect().height;
        await empty.unmount();
        mocks.activeStore = {
          serverInfo: { motd },
          voiceCall: { connected: false },
          attention: {
            counts: { unreadNotificationCount: 0, importantUnreadNotificationCount: 0 }
          }
        };
        const { container, getByRole } = render(AppHeader);
        const header = container.querySelector('header')!;
        await expect.element(getByRole('link', { name: 'https://chatto.run' })).toBeVisible();
        expect(header.getBoundingClientRect().height).toBe(emptyHeight);
        const headerBounds = header.getBoundingClientRect();
        expect(header.scrollWidth).toBeLessThanOrEqual(header.clientWidth);
        const about = container.querySelector<HTMLButtonElement>(
          'button[aria-label="About Chatto"]'
        )!;
        expect(about.textContent?.trim()).toBe('');
        expect(about.getBoundingClientRect().width).toBe(44);
        expect(about.getBoundingClientRect().height).toBe(44);
        for (const control of header.querySelectorAll<HTMLElement>('.app-header-icon')) {
          const bounds = control.getBoundingClientRect();
          expect(bounds.left).toBeGreaterThanOrEqual(headerBounds.left);
          expect(bounds.right).toBeLessThanOrEqual(headerBounds.right);
        }
        const preview = container.querySelector<HTMLElement>('[data-testid="motd-preview"]')!;
        expect(preview.scrollWidth).toBeGreaterThan(preview.clientWidth);
        expect(getComputedStyle(preview).textOverflow).toBe('ellipsis');
        await getByRole('button', { name: 'Message of the Day' }).click({
          position: { x: 4, y: 4 }
        });
        expect(mocks.pushState).toHaveBeenCalledWith('', { modal: { type: 'motd', motd } });
      } finally {
        await page.viewport(1280, 720);
      }
    }
  );
});
