import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { SvelteMap } from 'svelte/reactivity';
import { sidebarNav } from '$lib/state/globals.svelte';

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
  stores: new Map<string, StoreMock>(),
  routeId: '/chat/-/overview',
  pageState: {} as App.PageState,
  pushState: vi.fn()
}));

vi.mock('$app/state', () => ({
  page: {
    get route() {
      return { id: mocks.routeId };
    },
    get state() {
      return mocks.pageState;
    }
  }
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState }));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    tryGetStore: (serverId: string) => mocks.stores.get(serverId)
  }
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
  mocks.routeId = '/chat/-/overview';
  mocks.pageState = {};
  mocks.pushState.mockReset();
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

  it('links the add action to the full Server Directory page', () => {
    mocks.routeId = '/chat/servers';

    const { container } = render(ServerGutter);
    const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]');

    expect(link?.getAttribute('aria-current')).toBe('page');
    expect(link?.className).toContain('server-gutter-item-active');
  });

  /**
   * Click the add action and report whether the gutter cancelled the default
   * link navigation. A window listener runs after Svelte's delegated handler,
   * records its decision, and then stops the test frame from navigating.
   */
  function clickAddServer(container: HTMLElement, init: MouseEventInit = {}): boolean {
    const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]')!;
    let cancelledByGutter = false;
    const observe = (event: Event) => {
      cancelledByGutter = event.defaultPrevented;
      event.preventDefault();
    };
    window.addEventListener('click', observe, { once: true });
    link.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, ...init }));
    window.removeEventListener('click', observe);
    return cancelledByGutter;
  }

  it('opens the Server Directory as a view over the current route', () => {
    const { container } = render(ServerGutter);

    expect(clickAddServer(container)).toBe(true);
    expect(mocks.pushState).toHaveBeenCalledWith('', { modal: { type: 'addServer' } });
  });

  it.each([
    ['a Meta click', { metaKey: true }],
    ['a Control click', { ctrlKey: true }],
    ['a Shift click', { shiftKey: true }],
    ['an Alt click', { altKey: true }],
    ['a middle-button click', { button: 1 }]
  ])('keeps %s as an ordinary link', (_name, init) => {
    const { container } = render(ServerGutter);

    expect(clickAddServer(container, init)).toBe(false);
    expect(mocks.pushState).not.toHaveBeenCalled();
  });

  it('marks the add action active and only closes the drawer while the view is open', () => {
    mocks.pageState = { modal: { type: 'addServer' } };
    sidebarNav.setMobile(true);
    sidebarNav.toggle();

    try {
      const { container } = render(ServerGutter);
      const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]')!;

      expect(link.getAttribute('aria-current')).toBe('page');
      expect(clickAddServer(container)).toBe(true);
      expect(mocks.pushState).not.toHaveBeenCalled();
      expect(sidebarNav.isOpen).toBe(false);
    } finally {
      sidebarNav.setMobile(false);
    }
  });

  it('keeps the link behavior on the Server Directory page', () => {
    mocks.routeId = '/chat/servers';

    const { container } = render(ServerGutter);

    expect(clickAddServer(container)).toBe(false);
    expect(mocks.pushState).not.toHaveBeenCalled();
  });
});
