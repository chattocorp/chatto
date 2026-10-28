import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { SvelteMap } from 'svelte/reactivity';
import { prefersReducedMotion } from 'svelte/motion';

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

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState, goto: vi.fn() }));

vi.mock('./ServerSidebarEntry.svelte', async () => ({
  default: (await import('./ServerGutterEntryMock.svelte')).default
}));

vi.mock('$lib/ui/ScrollFader.svelte', async () => ({
  default: (await import('./ServerGutterScrollFaderMock.svelte')).default
}));

import ServerGutter from './ServerGutter.svelte';
import { serverGutterOrder } from './state/serverGutterOrder.svelte';
import { tick } from 'svelte';
import { TRIGGERS, SOURCES, SHADOW_PLACEHOLDER_ITEM_ID } from 'svelte-dnd-action';
import type { GutterItem } from './serverGutterDrag.svelte';

beforeEach(() => {
  serverGutterOrder.save([]);
  mocks.servers = [];
  mocks.stores = new SvelteMap();
  mocks.routeId = '/chat/-/overview';
  mocks.pageState = {};
  mocks.pushState.mockReset();
});

afterEach(() => vi.restoreAllMocks());

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

  it('fades added and removed entries and resumes dragging after removal', async () => {
    vi.spyOn(prefersReducedMotion, 'current', 'get').mockReturnValue(false);
    register('origin', 'a', 'b');
    mocks.stores.delete('b');
    const { container } = render(ServerGutter);
    const zone = container.querySelector<HTMLElement>('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.dataset.serverDragReady).toBe('true'));

    mocks.stores.set('b', { isAuthenticated: false, currentUser: {} });
    await tick();
    const entry = [...zone.querySelectorAll('[data-testid="server-entry"]')].find(
      (node) => node.textContent === 'b'
    )!;
    const wrapper = entry.parentElement!;
    await vi.waitFor(() =>
      expect(wrapper.getAnimations()[0]?.effect?.getTiming().duration).toBe(150)
    );
    await Promise.all(wrapper.getAnimations().map((animation) => animation.finished));

    mocks.stores.delete('b');
    await tick();
    expect(entry.isConnected).toBe(true);
    expect(wrapper.inert).toBe(true);
    expect(zone.dataset.serverDragReady).toBeUndefined();
    await vi.waitFor(() => expect(wrapper.getAnimations().length).toBeGreaterThan(0));
    await Promise.all(wrapper.getAnimations().map((animation) => animation.finished));
    await vi.waitFor(() => expect(entry.isConnected).toBe(false));
    await vi.waitFor(() => expect(zone.dataset.serverDragReady).toBe('true'));
    expect(entries(container)).toEqual(['origin', 'a']);
  });

  it('removes entries immediately with reduced motion', async () => {
    vi.spyOn(prefersReducedMotion, 'current', 'get').mockReturnValue(true);
    register('origin', 'a');
    const { container } = render(ServerGutter);
    mocks.stores.delete('a');
    await tick();
    expect(entries(container)).toEqual(['origin']);
    expect(container.getAnimations({ subtree: true })).toHaveLength(0);
  });

  it('restores dragging when a server returns before its fade-out finishes', async () => {
    vi.spyOn(prefersReducedMotion, 'current', 'get').mockReturnValue(false);
    register('origin', 'a', 'b');
    const store = mocks.stores.get('b')!;
    const { container } = render(ServerGutter);
    const zone = container.querySelector<HTMLElement>('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.dataset.serverDragReady).toBe('true'));
    mocks.stores.delete('b');
    await tick();
    expect(zone.dataset.serverDragReady).toBeUndefined();
    mocks.stores.set('b', store);
    await tick();
    await vi.waitFor(() => expect(zone.dataset.serverDragReady).toBe('true'));
    expect(entries(container)).toEqual(['origin', 'a', 'b']);
  });

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

  it.each([
    [
      'duplicate keys',
      [
        { id: 'a', serverId: 'a' },
        { id: 'a', serverId: 'origin' }
      ]
    ],
    [
      'duplicate servers',
      [
        { id: 'a', serverId: 'a' },
        { id: 'other', serverId: 'a' }
      ]
    ],
    ['missing servers', [{ id: 'a', serverId: 'a' }]],
    [
      'unknown servers',
      [
        { id: 'a', serverId: 'a' },
        { id: 'other', serverId: 'other' }
      ]
    ],
    [
      'multiple shadows',
      [
        { id: 'a', serverId: 'a', isDndShadowItem: true },
        { id: 'origin', serverId: 'origin', isDndShadowItem: true }
      ]
    ]
  ])('retains the last valid preview after %s', async (_name, invalidItems) => {
    register('origin', 'a');
    const { container } = render(ServerGutter);
    const zone = container.querySelector('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.getAttribute('data-server-drag-ready')).toBe('true'));
    const info = { id: 'origin', source: SOURCES.POINTER, trigger: TRIGGERS.DRAG_STARTED };
    zone.dispatchEvent(
      new CustomEvent('consider', {
        detail: {
          items: [
            { id: 'a', serverId: 'a' },
            { id: 'origin', serverId: 'origin' }
          ],
          info
        }
      })
    );
    await tick();
    zone.dispatchEvent(new CustomEvent('consider', { detail: { items: invalidItems, info } }));
    await tick();
    expect(entries(container)).toEqual(['a', 'origin']);
    expect(zone.querySelectorAll('[data-is-dnd-shadow-item-hint]')).toHaveLength(0);
    expect(JSON.parse(localStorage.getItem('chatto:serverGutterOrder')!)).toEqual([]);
  });

  it('copies a shadow preview before the drag library changes its key', async () => {
    register('origin', 'a');
    const { container } = render(ServerGutter);
    const zone = container.querySelector('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.getAttribute('data-server-drag-ready')).toBe('true'));
    const items: GutterItem[] = [
      { id: 'a', serverId: 'a' },
      { id: SHADOW_PLACEHOLDER_ITEM_ID, serverId: 'origin', isDndShadowItem: true }
    ];
    const info = { id: 'origin', source: SOURCES.POINTER, trigger: TRIGGERS.DRAG_STARTED };
    zone.dispatchEvent(new CustomEvent('consider', { detail: { items, info } }));
    await tick();
    const shadow = zone.querySelector('[data-is-dnd-shadow-item-hint]');
    expect(shadow).not.toBeNull();

    // The library changes shadow IDs in place after removing the original node.
    items[1].id = 'origin';
    items[1].serverId = 'a';
    items.reverse();
    mocks.stores.set('origin', { isAuthenticated: true, currentUser: {} });
    await tick();
    expect(entries(container)).toEqual(['a', 'origin']);
    expect(zone.querySelector('[data-is-dnd-shadow-item-hint]')).toBe(shadow);

    // A later event can publish the library's new shadow key explicitly.
    zone.dispatchEvent(
      new CustomEvent('consider', {
        detail: {
          items: [
            { id: 'a', serverId: 'a' },
            { id: 'origin', serverId: 'origin', isDndShadowItem: true }
          ],
          info: { ...info, trigger: TRIGGERS.DRAGGED_OVER_INDEX }
        }
      })
    );
    await tick();
    expect(entries(container)).toEqual(['a', 'origin']);
    expect(zone.querySelectorAll('[data-is-dnd-shadow-item-hint]')).toHaveLength(1);
    expect(zone.querySelector('[data-is-dnd-shadow-item-hint]')).not.toBe(shadow);

    const dropped = [
      { id: 'a', serverId: 'a' },
      { id: 'origin', serverId: 'origin' }
    ];
    zone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: { items: dropped, info: { ...info, trigger: TRIGGERS.DROPPED_INTO_ZONE } }
      })
    );
    await tick();
    expect(entries(container)).toEqual(['a', 'origin']);
    expect(zone.querySelector('[data-is-dnd-shadow-item-hint]')).toBeNull();
    expect(JSON.parse(localStorage.getItem('chatto:serverGutterOrder')!)).toEqual(['a', 'origin']);
  });

  it.each([
    ['duplicate servers', ['a', 'a'], TRIGGERS.DROPPED_INTO_ZONE],
    ['missing servers', ['a'], TRIGGERS.DROPPED_INTO_ZONE],
    ['unknown servers', ['a', 'unknown'], TRIGGERS.DROPPED_INTO_ZONE],
    ['an outside drop', ['a', 'origin'], TRIGGERS.DROPPED_OUTSIDE_OF_ANY]
  ])('restores the committed order after %s', async (_name, ids, trigger) => {
    register('origin', 'a');
    const { container } = render(ServerGutter);
    const zone = container.querySelector('[data-testid="server-list"]')!;
    await vi.waitFor(() => expect(zone.getAttribute('data-server-drag-ready')).toBe('true'));
    const info = { id: 'origin', source: SOURCES.POINTER, trigger: TRIGGERS.DRAG_STARTED };
    zone.dispatchEvent(
      new CustomEvent('consider', {
        detail: { items: ['a', 'origin'].map((id) => ({ id, serverId: id })), info }
      })
    );
    await tick();
    zone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: { items: ids.map((id) => ({ id, serverId: id })), info: { ...info, trigger } }
      })
    );
    await tick();
    expect(entries(container)).toEqual(['origin', 'a']);
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
    expect(link?.className).not.toContain('server-gutter-item-active');
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

  it('navigates to the Server Directory through an ordinary link', () => {
    const { container } = render(ServerGutter);

    expect(clickAddServer(container)).toBe(false);
    expect(mocks.pushState).not.toHaveBeenCalled();
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
});
