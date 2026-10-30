import { tick } from 'svelte';
import { page } from '$app/state';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import type { PublicServerInfo } from '@chatto/client/api/server';
import { sidebarNav } from '$lib/state/globals.svelte';
import { serverRegistry } from '$lib/client';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    invalidateAll: vi.fn(),
    afterNavigate: vi.fn(),
    beforeNavigate: vi.fn(),
    onNavigate: vi.fn(),
    deactivateManagementLocaleMessages: vi.fn(async () => {}),
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

vi.mock('$lib/i18n/messages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/i18n/messages')>()),
  deactivateManagementLocaleMessages: mocks.deactivateManagementLocaleMessages
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    servers: [],
    originServer: { id: 'origin' },
    getStore: vi.fn(),
    getServer: vi.fn(() => ({ userId: 'U1' })),
    tryGetStore: vi.fn(() => null),
    isAuthenticated: vi.fn(() => false)
  },
  serverConnectionManager: {
    originClient: mocks.originClient,
    getClient: vi.fn(() => mocks.originClient)
  }
}));

vi.mock('$lib/serverCatalogue', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/serverCatalogue')>()),
  firstAuthenticatedServerId: vi.fn(() => undefined)
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
  refreshPushSubscriptions: vi.fn(),
  unsubscribeBeforeLeaving: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/notifications/notificationClick', () => ({
  onNotificationClick: vi.fn(() => vi.fn())
}));

vi.mock('$lib/notifications/notificationNavigationUi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/notifications/notificationNavigationUi')>()),
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

function renderLayout(content = '<main data-testid="layout-child"></main>') {
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
      data: { serverInfo, serverInfoLoaded: true, user: null },
      children: testSnippet(content)
    }
  });
}

function touch(type: string, x: number, y = 120) {
  const contact = new Touch({ identifier: 1, target: document.body, clientX: x, clientY: y });
  return new TouchEvent(type, {
    bubbles: true,
    cancelable: true,
    touches: type === 'touchend' ? [] : [contact],
    changedTouches: [contact]
  });
}

describe('main-view navigation fade', () => {
  it('fades channel and page changes but leaves thread and query navigation steady', () => {
    mocks.onNavigate.mockClear();
    renderLayout();
    const navigate = mocks.onNavigate.mock.calls[0][0];
    const room = {
      url: new URL('https://chat.example.test/chat/server/room'),
      route: { id: '/chat/[serverId]/[roomId]' },
      params: { serverId: 'server', roomId: 'room' }
    };
    expect(navigate({ from: null, to: room })).toBeUndefined();
    expect(navigate({ from: room, to: null })).toBeUndefined();
    expect(
      navigate({ from: room, to: { ...room, url: new URL(`${room.url}?filter=1`) } })
    ).toBeUndefined();
    expect(
      navigate({
        from: room,
        to: {
          ...room,
          url: new URL(`${room.url}/thread`),
          route: { id: '/chat/[serverId]/[roomId]/[threadId]' }
        }
      })
    ).toBeUndefined();
    expect(
      navigate({
        from: room,
        to: {
          ...room,
          url: new URL('https://chat.example.test/chat/server/other'),
          params: { ...room.params, roomId: 'other' }
        }
      })
    ).toBeTypeOf('function');
    expect(
      navigate({
        from: room,
        to: {
          url: new URL('https://chat.example.test/chat/server/manage/server'),
          route: { id: '/chat/[serverId]/manage/server' },
          params: { serverId: 'server' }
        }
      })
    ).toBeTypeOf('function');
  });
});

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
    child.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));
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

describe('room member navigation boundary', () => {
  it('requests members on direct entry before the room mounts, and on room changes', async () => {
    const loadRoomMembers = vi.fn();
    vi.mocked(serverRegistry.tryGetStore).mockReturnValue({
      loadRoomMembers,
      serverInfo: {},
      currentUser: { user: null }
    } as never);
    page.route.id = '/chat/[serverId]/[roomId]';
    Object.assign(page, { params: { serverId: '-', roomId: 'room-1' } });
    try {
      renderLayout();
      const navigate = mocks.afterNavigate.mock.calls.at(-1)![0];
      await navigate({ from: null, to: { route: page.route } });
      expect(loadRoomMembers).toHaveBeenLastCalledWith('room-1');

      page.route.id = '/chat/[serverId]/[roomId]/[threadId]';
      Object.assign(page, { params: { serverId: '-', roomId: 'room-2', threadId: 'thread' } });
      await tick();
      await navigate({ from: null, to: { route: page.route } });
      expect(loadRoomMembers).toHaveBeenLastCalledWith('room-2');
      expect(loadRoomMembers).toHaveBeenCalledTimes(2);
    } finally {
      page.route.id = '/';
      Object.assign(page, { params: {} });
      vi.mocked(serverRegistry.tryGetStore).mockReturnValue(undefined);
    }
  });
});

describe('management catalog navigation boundary', () => {
  beforeEach(() => vi.clearAllMocks());

  it.each([
    ['/chat/[serverId]/manage/server/general', '/chat/[serverId]/settings/language', true],
    ['/chat/[serverId]/manage/rooms', '/login', true],
    ['/chat/[serverId]/manage/server/general', '/chat/[serverId]/manage/rooms', false],
    ['/chat/[serverId]/settings/appearance', '/chat/[serverId]/settings/language', false],
    ['/chat/[serverId]/[roomId]', '/chat/[serverId]/[roomId]/thread/[threadId]', false],
    ['/chat/[serverId]/settings/appearance', '/chat/[serverId]/manage/server/general', false]
  ])('updates catalogs only after exiting management: %s -> %s', async (from, to, exits) => {
    renderLayout();
    const callback = mocks.afterNavigate.mock.calls.at(-1)![0];
    // Registration must not change sections before navigation completes.
    expect(mocks.deactivateManagementLocaleMessages).not.toHaveBeenCalled();
    await callback({ from: { route: { id: from } }, to: { route: { id: to } } });
    expect(mocks.deactivateManagementLocaleMessages).toHaveBeenCalledTimes(exits ? 1 : 0);
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

    child.dispatchEvent(touch('touchstart', 2));
    window.dispatchEvent(touch('touchend', 2));
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

    child.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));
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

    panel.dispatchEvent(touch('touchstart', 320));
    window.dispatchEvent(touch('touchmove', 0));
    window.dispatchEvent(touch('touchend', 0));
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

    child.dispatchEvent(touch('touchstart', 310));
    window.dispatchEvent(touch('touchmove', 100));
    window.dispatchEvent(touch('touchend', 100));
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
