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
  stores: new Map<string, StoreMock>(),
  routeId: '/chat/-/overview',
  pageState: {} as App.PageState,
  pushState: vi.fn()
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    tryGetStore: (serverId: string) => mocks.stores.get(serverId),
    isOriginServer: (serverId: string) => serverId === 'origin'
  }
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

vi.mock('./ServerSidebarEntry.svelte', async () => ({
  default: (await import('./ServerGutterEntryMock.svelte')).default
}));

vi.mock('$lib/ui/ScrollFader.svelte', async () => ({
  default: (await import('./ServerGutterScrollFaderMock.svelte')).default
}));

import ServerGutter from './ServerGutter.svelte';
import { serverGutterOrder } from './state/serverGutterOrder.svelte';
import { tick } from 'svelte';
import { TRIGGERS, SOURCES } from 'svelte-dnd-action';

beforeEach(() => {
  serverGutterOrder.save([]);
  mocks.servers = [];
  mocks.stores = new SvelteMap();
  mocks.routeId = '/chat/-/overview';
  mocks.pageState = {};
  mocks.pushState.mockReset();
});

describe('ServerGutter', () => {
  function register(...ids: string[]) {
    mocks.servers = ids.map((id) => ({ id, reauthRequiredAt: null }));
    ids.forEach((id) => mocks.stores.set(id, { isAuthenticated: false, currentUser: {} }));
  }

  function entries(container: HTMLElement) {
    return [...container.querySelectorAll('[data-testid="server-entry"]')].map(
      (entry) => entry.textContent
    );
  }

  it('keeps the existing display order when saved order omits the formerly pinned origin', () => {
    register('a', 'b', 'origin', 'new');
    serverGutterOrder.save(['b', 'missing', 'b', 'a']);
    const { container } = render(ServerGutter);
    expect(entries(container)).toEqual(['origin', 'b', 'a', 'new']);
  });

  it('applies the saved position of the origin within the shared server list', () => {
    register('origin', 'a', 'b', 'new');
    serverGutterOrder.save(['b', 'origin', 'a']);
    const { container } = render(ServerGutter);
    expect(entries(container)).toEqual(['b', 'origin', 'a', 'new']);
  });

  it('updates on a storage event without remounting entries or writing back', async () => {
    register('origin', 'a', 'b');
    const { container } = render(ServerGutter);
    const original = [...container.querySelectorAll('[data-testid="server-entry"]')];
    localStorage.setItem('chatto:serverGutterOrder', JSON.stringify(['b', 'origin', 'a']));
    const write = vi.spyOn(Storage.prototype, 'setItem');
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: 'chatto:serverGutterOrder',
        storageArea: localStorage,
        newValue: JSON.stringify(['origin', 'a', 'b']) // An older queued event must not win.
      })
    );
    await tick();
    expect(entries(container)).toEqual(['b', 'origin', 'a']);
    expect(container.querySelectorAll('[data-testid="server-entry"]')[1]).toBe(original[0]);
    expect(container.querySelectorAll('[data-testid="server-entry"]')[2]).toBe(original[1]);
    expect(write).not.toHaveBeenCalled();
    write.mockRestore();
  });

  it('keeps drag previews local and saves only a complete drop', async () => {
    register('origin', 'a');
    const { container } = render(ServerGutter);
    const zone = container.querySelector('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.getAttribute('data-server-drag-ready')).toBe('true'));
    const items = ['a', 'origin'].map((id) => ({ id, serverId: id }));
    const info = { id: 'origin', source: SOURCES.POINTER, trigger: TRIGGERS.DRAG_STARTED };
    zone.dispatchEvent(new CustomEvent('consider', { detail: { items, info } }));
    await tick();
    expect(entries(container)).toEqual(['a', 'origin']);
    expect(JSON.parse(localStorage.getItem('chatto:serverGutterOrder')!)).toEqual([]);
    zone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: { items, info: { ...info, trigger: TRIGGERS.DROPPED_INTO_ZONE } }
      })
    );
    await tick();
    expect(JSON.parse(localStorage.getItem('chatto:serverGutterOrder')!)).toEqual(['a', 'origin']);
  });

  it('discards a drop after catalogue membership changes', async () => {
    register('origin', 'a');
    const { container } = render(ServerGutter);
    const zone = container.querySelector('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.getAttribute('data-server-drag-ready')).toBe('true'));
    const items = ['a', 'origin'].map((id) => ({ id, serverId: id }));
    const info = { id: 'origin', source: SOURCES.POINTER, trigger: TRIGGERS.DRAG_STARTED };
    zone.dispatchEvent(new CustomEvent('consider', { detail: { items, info } }));
    mocks.stores.delete('a');
    await tick();
    zone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: { items, info: { ...info, trigger: TRIGGERS.DROPPED_INTO_ZONE } }
      })
    );
    await tick();
    expect(entries(container)).toEqual(['origin']);
    expect(JSON.parse(localStorage.getItem('chatto:serverGutterOrder')!)).toEqual([]);
  });

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

  it('opens the Server Directory as a dialog over the current view', () => {
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

  it('marks the add action active while the dialog is open', () => {
    mocks.pageState = { modal: { type: 'addServer' } };

    const { container } = render(ServerGutter);
    const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]')!;

    expect(link.getAttribute('aria-current')).toBe('page');
    expect(clickAddServer(container)).toBe(false);
    expect(mocks.pushState).not.toHaveBeenCalled();
  });
});
