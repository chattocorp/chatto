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

  it('opens the Server Directory as a dialog over the current view', () => {
    const { container } = render(ServerGutter);
    const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]')!;

    link.click();

    expect(mocks.pushState).toHaveBeenCalledWith('', { modal: { type: 'addServer' } });
  });

  it('keeps modified clicks as ordinary links', () => {
    const { container } = render(ServerGutter);
    const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]')!;
    const event = new MouseEvent('click', { bubbles: true, cancelable: true, metaKey: true });
    // Keep the browser from following the link during the test.
    link.addEventListener('click', (clickEvent) => clickEvent.preventDefault(), { once: true });

    link.dispatchEvent(event);

    expect(mocks.pushState).not.toHaveBeenCalled();
  });

  it('marks the add action active while the dialog is open', () => {
    mocks.pageState = { modal: { type: 'addServer' } };

    const { container } = render(ServerGutter);
    const link = container.querySelector<HTMLAnchorElement>('a[href="/chat/servers"]')!;

    expect(link.getAttribute('aria-current')).toBe('page');
    link.click();
    expect(mocks.pushState).not.toHaveBeenCalled();
  });
});
