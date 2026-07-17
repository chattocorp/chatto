import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import type { PublicServerInfo } from '$lib/api-client/server';
import { sidebarNav } from '$lib/state/globals.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    afterNavigate: vi.fn(),
    onNavigate: vi.fn(),
    appUi: {
      setActiveRoomScope: vi.fn(),
      setActiveServer: vi.fn()
    },
    originClient: {
      showConnectionLostIcon: false,
      showConnectionLostBanner: false,
      forceReconnect: vi.fn()
    }
  }
}));

vi.mock('$app/navigation', () => ({
  afterNavigate: mocks.afterNavigate,
  goto: mocks.goto,
  onNavigate: mocks.onNavigate,
  pushState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$app/state', () => ({
  page: {
    params: {},
    route: { id: '/' },
    state: {},
    url: new URL('https://chat.example.test/')
  },
  updated: {
    current: false
  }
}));

vi.mock('$lib/hooks', () => ({
  usePageTitle: () => () => 'Chatto',
  usePinchZoomPrevention: vi.fn(),
  useVisualViewport: vi.fn()
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  onNotificationClick: vi.fn(() => vi.fn())
}));

vi.mock('$lib/notifications/notificationNavigationUi', () => ({
  prepareUiForNotificationPath: vi.fn(),
  prepareUiForNotificationTarget: vi.fn()
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    servers: [],
    originServer: { id: 'origin' },
    getStore: vi.fn(),
    tryGetStore: vi.fn(() => null)
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

function renderLayout() {
  const serverInfo: PublicServerInfo = {
    name: 'Test Server',
    version: 'test',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    welcomeMessage: null,
    description: null,
    iconUrl: null,
    bannerUrl: null,
    authProviders: [],
    compatibility: {
      protocolCapabilities: [
        'chatto.api.v1',
        'chatto.realtime.v1',
        'chatto.realtime.projection.v1'
      ],
      minimumWebClientVersion: null
    }
  };

  return render(Layout, {
    props: {
      data: {
        serverInfo,
        serverInfoLoaded: true,
        user: null
      },
      children: testSnippet('<main data-testid="layout-child"></main>')
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

describe('root layout mobile sidebar animation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    installMobileMatchMedia();
    resetSidebar();
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
    expect(q(container, '[data-testid="mobile-sidebar-panel"]')?.style.transform).toBe(
      'translateX(0px)'
    );
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

    expect(panel.style.transform).toBe('translateX(0px)');
    expect(getComputedStyle(panel).visibility).toBe('visible');
    expect(backdrop.disabled).toBe(false);
    expect(backdrop.style.opacity).toBe('1');

    backdrop.click();
    await tick();

    expect(q(container, '[data-testid="mobile-sidebar-backdrop"]')).toBe(backdrop);
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.style.opacity).toBe('0');
    expect(panel.style.transform).toBe('translateX(-324px)');
    expect(panel.classList.contains('sidebar-mobile-closed')).toBe(true);
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
    expect(panel.style.transform).toBe('translateX(-324px)');
  });
});
