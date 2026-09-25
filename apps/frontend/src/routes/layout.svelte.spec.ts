import { tick } from 'svelte';
import { page } from '$app/state';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import type { PublicServerInfo } from '$lib/api-client/server';
import { sidebarNav } from '$lib/state/globals.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    invalidateAll: vi.fn(),
    recoverServer: vi.fn(),
    afterNavigate: vi.fn(),
    beforeNavigate: vi.fn(),
    onNavigate: vi.fn(),
    appUi: {
      setActiveRoomScope: vi.fn(),
      setActiveServer: vi.fn()
    },
    originClient: {
      showConnectionLostIcon: false,
      showConnectionLostBanner: false,
      forceReconnect: vi.fn()
    },
    updateAppBadge: vi.fn(async () => {})
  }
}));

vi.mock('$app/navigation', () => ({
  afterNavigate: mocks.afterNavigate,
  beforeNavigate: mocks.beforeNavigate,
  goto: mocks.goto,
  invalidateAll: mocks.invalidateAll,
  onNavigate: mocks.onNavigate,
  pushState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$app/state', async () => {
  const { fromStore, writable } = await import('svelte/store');
  const routeId = fromStore(writable('/'));
  const url = fromStore(writable(new URL('https://chat.example.test/')));
  return {
    page: {
      params: {},
      route: {
        get id() {
          return routeId.current;
        },
        set id(value: string) {
          routeId.current = value;
        }
      },
      state: {},
      get url() {
        return url.current;
      },
      set url(value: URL) {
        url.current = value;
      }
    },
    updated: { current: false }
  };
});

vi.mock('$lib/hooks/usePageTitle.svelte', () => ({
  usePageTitle: () => () => 'Chatto'
}));

vi.mock('$lib/hooks/usePinchZoomPrevention.svelte', () => ({
  usePinchZoomPrevention: vi.fn()
}));

vi.mock('$lib/hooks/useVisualViewport.svelte', () => ({
  useVisualViewport: vi.fn()
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  enablePushOnAllServers: vi.fn().mockResolvedValue({ permission: null, registrations: [] }),
  getPermission: vi.fn(() => null),
  getPushCapability: vi.fn(() => 'unsupported'),
  getPushRegistrationTargets: vi.fn(() => []),
  onNotificationClick: vi.fn(() => vi.fn()),
  refreshPushSubscriptions: vi.fn(),
  unsubscribeBeforeLeaving: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/notifications/notificationNavigationUi', () => ({
  prepareUiForNotificationPath: vi.fn(),
  prepareUiForNotificationTarget: vi.fn()
}));

vi.mock('$lib/notifications/appBadge', () => ({
  listenForAppBadgeRefresh: vi.fn(() => vi.fn()),
  updateAppBadge: mocks.updateAppBadge
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi,
  provideAppUiState: () => mocks.appUi
}));

vi.mock('$lib/state/server/useServerRegistry.svelte', () => ({
  useServerRegistry: vi.fn()
}));

vi.mock('$lib/state/server/ServerRuntimeCoordinator.svelte', async () => ({
  default: (await import('./chat/ChatRootTestStub.svelte')).default
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  generateServerId: vi.fn(() => 'server-id'),
  serverRegistry: {
    servers: [],
    originServer: { id: 'origin' },
    getStore: vi.fn(),
    getServer: vi.fn(() => ({ userId: 'U1' })),
    tryGetStore: vi.fn(() => null),
    recoverServer: mocks.recoverServer,
    isAuthenticated: vi.fn(() => false),
    firstAuthenticatedServerId: vi.fn(() => undefined)
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    originClient: mocks.originClient,
    getClient: vi.fn(() => mocks.originClient)
  }
}));

import Layout from './+layout.svelte';

function installMobileMatchMedia() {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn(() => ({
      matches: true,
      media: '(max-width: 767px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  });
}

function resetSidebar() {
  sidebarNav.setMobile(false);
  if (!sidebarNav.isOpen) sidebarNav.toggle();
  sidebarNav.setMobile(true);
}

function renderLayout(
  content = '<main data-testid="layout-child"></main>',
  startup: { startupPending: boolean; startupServerId: string } | null = null
) {
  const serverInfo: PublicServerInfo = {
    name: 'Test Server',
    version: 'test',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'open',
    welcomeMessage: null,
    description: null,
    iconUrl: null,
    bannerUrl: null,
    authProviders: []
  };

  return render(Layout, {
    props: {
      data: startup
        ? { serverInfo: null, serverInfoLoaded: false, user: null, ...startup }
        : { serverInfo, serverInfoLoaded: true, user: null },
      children: testSnippet(content)
    }
  });
}

function pointer(type: string, x: number, y = 120) {
  return new PointerEvent(type, {
    bubbles: true,
    cancelable: true,
    pointerId: 1,
    clientX: x,
    clientY: y
  });
}

describe('OAuth page layout', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(serverRegistry.tryGetStore).mockReturnValue(undefined);
    installMobileMatchMedia();
    resetSidebar();
  });

  afterEach(() => {
    page.route.id = '/';
    Object.assign(page, { url: new URL('https://chat.example.test/') });
  });

  it('verifies a saved viewer after paint without remounting the route', async () => {
    const startupStore = $state({
      startupPresentationOnly: true,
      currentUser: { user: undefined },
      serverInfo: { motd: null },
      notifications: {
        attention: { unreadNotificationCount: 0, importantUnreadNotificationCount: 0 }
      }
    });
    vi.mocked(mocks.recoverServer).mockImplementation(async () => {
      startupStore.startupPresentationOnly = false;
    });
    vi.mocked(serverRegistry.tryGetStore).mockReturnValue(startupStore as never);
    const view = renderLayout(
      '<main data-testid="layout-child"><div data-testid="timeline" style="height: 20px; overflow: auto"><div style="height: 100px">Message</div></div><input data-testid="composer" /></main>',
      { startupPending: true, startupServerId: 'origin' }
    );
    const child = q(view.container, '[data-testid="layout-child"]')!;
    const timeline = q(view.container, '[data-testid="timeline"]')!;
    const composer = q(view.container, '[data-testid="composer"]') as HTMLInputElement;
    timeline.scrollTop = 24;
    composer.value = 'draft text';

    await vi.waitFor(() => expect(mocks.recoverServer).toHaveBeenCalledWith('origin'));
    await tick();

    expect(q(view.container, '[data-testid="layout-child"]')).toBe(child);
    expect(q(view.container, '[data-testid="timeline"]')).toBe(timeline);
    expect(q(view.container, '[data-testid="composer"]')).toBe(composer);
    expect(timeline.scrollTop).toBe(24);
    expect(composer.value).toBe('draft text');
    expect(mocks.invalidateAll).not.toHaveBeenCalled();
  });

  it('restores the frame when client-side navigation leaves OAuth login', async () => {
    page.route.id = '/login';
    Object.assign(page, {
      url: new URL('https://chat.example.test/login?redirect=%2Foauth%2Fauthorize')
    });
    const view = renderLayout();
    await expect.element(view.getByTestId('app-frame')).not.toBeInTheDocument();

    Object.assign(page, { url: new URL('https://chat.example.test/login') });
    await expect.element(view.getByTestId('app-frame')).toBeInTheDocument();

    page.route.id = '/oauth/consent';
    await expect.element(view.getByTestId('app-frame')).not.toBeInTheDocument();
    page.route.id = '/register';
    await expect.element(view.getByTestId('app-frame')).toBeInTheDocument();
  });

  it.each([
    ['/oauth/consent', ''],
    ['/servers/callback', '?mode=popup'],
    ['/servers/callback', '?mode=provider'],
    ['/login', '?redirect=%2Foauth%2Fauthorize'],
    ['/login', '?redirect=%2Foauth%2Fauthorize%3Fclient_id%3Dtest'],
    ['/login', '?redirect=%2Foauth%2Fconsent']
  ] as const)('renders %s%s without app navigation', async (route, search) => {
    page.route.id = route;
    Object.assign(page, { url: new URL(route + search, 'https://chat.example.test') });
    const view = renderLayout(
      '<main data-testid="layout-child"><p role="status">Loading</p><p role="alert">Request failed</p></main>'
    );

    await expect.element(view.getByText('Loading', { exact: true })).toBeVisible();
    await expect.element(view.getByRole('alert')).toBeVisible();
    expect(q(view.container, '[data-testid="app-frame"]')).toBeNull();
    expect(q(view.container, '[data-testid="mobile-sidebar-panel"]')).toBeNull();
    await expect
      .element(view.getByRole('button', { name: 'Toggle sidebar' }))
      .not.toBeInTheDocument();

    const child = q(view.container, '[data-testid="layout-child"]')!;
    child.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));
    await tick();
    expect(sidebarNav.isOpen).toBe(false);
  });

  it.each([
    ['/login', ''],
    ['/register', ''],
    ['/forgot-password', ''],
    ['/setup', ''],
    ['/chat', ''],
    ['/servers/callback', ''],
    ['/servers/callback', '?mode=unknown'],
    ['/login', '?redirect=%2Fchat'],
    ['/login', '?redirect=https%3A%2F%2Fexternal.test%2Foauth%2Fauthorize'],
    ['/login', '?redirect=%2F%2Fexternal.test%2Foauth%2Fauthorize'],
    ['/login', '?redirect=%2F%5Cexternal.test%2Foauth%2Fauthorize'],
    ['/login', '?redirect=%2Foauth%2Fauthorize-other']
  ] as const)('keeps the app frame on %s%s', async (route, search) => {
    page.route.id = route;
    Object.assign(page, { url: new URL(route + search, 'https://chat.example.test') });
    const view = renderLayout();
    await expect.element(view.getByTestId('app-frame')).toBeInTheDocument();
    await expect.element(view.getByRole('button', { name: 'Toggle sidebar' })).toBeVisible();
  });
});

describe('root layout mobile sidebar animation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.documentElement.dir = 'ltr';
    installMobileMatchMedia();
    resetSidebar();
  });

  it('keeps the app header and server navigation available during setup', async () => {
    page.route.id = '/setup';
    try {
      const view = renderLayout();
      await expect.element(view.getByRole('button', { name: 'Toggle sidebar' })).toBeVisible();
      await view.getByRole('button', { name: 'Toggle sidebar' }).click();
      await expect.element(view.getByRole('link', { name: 'Add Server' })).toBeVisible();
    } finally {
      page.route.id = '/';
    }
  });

  it('keeps the left edge free for normal app controls', async () => {
    const { container } = renderLayout();
    await tick();

    const child = q(container, '[data-testid="layout-child"]');
    expect(child).not.toBeNull();
    if (!child) return;
    const onClick = vi.fn();
    child.addEventListener('click', onClick);

    child.dispatchEvent(pointer('pointerdown', 2));
    window.dispatchEvent(pointer('pointerup', 2));
    child.click();

    expect(q(container, '[data-testid="mobile-sidebar-edge"]')).toBeNull();
    expect(onClick).toHaveBeenCalledOnce();
    expect(sidebarNav.isOpen).toBe(false);
  });

  it('opens the mobile sidebar from a rightward drag in app content', async () => {
    const { container } = renderLayout();
    await tick();

    const child = q(container, '[data-testid="layout-child"]');
    expect(child).not.toBeNull();
    if (!child) return;

    child.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));
    await tick();

    expect(sidebarNav.isOpen).toBe(true);
    expect(q(container, '[data-testid="mobile-sidebar-panel"]')?.inert).toBe(false);
  });

  it('keeps the sidebar and backdrop mounted while the mobile close animation runs', async () => {
    const { container } = renderLayout();
    await tick();

    sidebarNav.toggle();
    await tick();

    const panel = q(container, '[data-testid="mobile-sidebar-panel"]');
    const backdrop = q(
      container,
      '[data-testid="mobile-sidebar-backdrop"]'
    ) as HTMLButtonElement | null;
    expect(panel).not.toBeNull();
    expect(backdrop).not.toBeNull();
    if (!panel || !backdrop) return;

    expect(panel.inert).toBe(false);
    expect(getComputedStyle(panel).visibility).toBe('visible');
    expect(backdrop.disabled).toBe(false);
    expect(backdrop.style.opacity).toBe('1');

    backdrop.click();
    await tick();

    expect(q(container, '[data-testid="mobile-sidebar-backdrop"]')).toBe(backdrop);
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.style.opacity).toBe('0');
    expect(panel.inert).toBe(true);
  });

  it('keeps drag-to-close working for the mobile sidebar', async () => {
    const { container } = renderLayout();
    await tick();

    sidebarNav.toggle();
    await tick();

    const panel = q(container, '[data-testid="mobile-sidebar-panel"]');
    expect(panel).not.toBeNull();
    if (!panel) return;

    panel.dispatchEvent(pointer('pointerdown', 320));
    window.dispatchEvent(pointer('pointermove', 0));
    window.dispatchEvent(pointer('pointerup', 0));
    await tick();

    expect(sidebarNav.isOpen).toBe(false);
    expect(panel.inert).toBe(true);
  });

  it('opens the inline-start sidebar from a leftward drag in RTL', async () => {
    document.documentElement.dir = 'rtl';
    const { container } = renderLayout();
    await tick();

    const child = q(container, '[data-testid="layout-child"]');
    expect(child).not.toBeNull();
    if (!child) return;

    child.dispatchEvent(pointer('pointerdown', 310));
    window.dispatchEvent(pointer('pointermove', 100));
    window.dispatchEvent(pointer('pointerup', 100));
    await tick();

    expect(sidebarNav.isOpen).toBe(true);
  });
});

describe('root layout notification synchronization', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    installMobileMatchMedia();
    resetSidebar();
  });

  it('mounts badge synchronization for a signed-out page', async () => {
    const { container } = renderLayout();

    await vi.waitFor(() => expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'clear' }));
    expect(container.querySelector('[data-testid="chat-root-component-stub"]')).not.toBeNull();
  });
});
