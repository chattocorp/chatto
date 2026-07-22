import { beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
import SearchPage from './+page.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    ensureStatus: vi.fn(),
    search: vi.fn(),
    loadMore: vi.fn(),
    goto: vi.fn(),
    activeServer: vi.fn(),
    serverStores: {} as Record<string, object>
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn()
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: mocks.activeServer }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: (serverId: string) => mocks.serverStores[serverId]
  }
}));

let activeServerId = $state('origin');

function serverStore(
  query = '',
  order = MessageSearchOrder.RELEVANCE,
  options: { nextCursor?: string | null; hasSearched?: boolean } = {}
) {
  const messageSearch = $state({
    status: { state: MessageSearchState.READY, retryAfterMs: null },
    statusLoading: false,
    statusLoaded: true,
    statusError: false,
    available: true,
    query,
    order,
    results: [],
    nextCursor: options.nextCursor ?? null,
    loading: false,
    loadingMore: false,
    error: false,
    hasSearched: options.hasSearched ?? false,
    ensureStatus: mocks.ensureStatus,
    refreshStatus: vi.fn(),
    search: mocks.search,
    loadMore: mocks.loadMore
  });
  return {
    currentUser: { user: { settings: null } },
    messageSearch
  };
}

describe('message search page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    activeServerId = 'origin';
    mocks.activeServer.mockImplementation(() => activeServerId);
    mocks.serverStores = { origin: serverStore(), remote: serverStore() };
  });

  it('mounts as a server page and submits an unscoped search', async () => {
    const { container } = render(SearchPage);

    const input = container.querySelector('input') as HTMLInputElement;
    await userEvent.type(input, 'motherfucking search');
    await userEvent.click(
      [...container.querySelectorAll('button')].find((button) => button.textContent?.trim() === 'Search')!
    );

    expect(container.textContent).toContain('Search messages');
    expect(mocks.ensureStatus).toHaveBeenCalledOnce();
    expect(mocks.search).toHaveBeenCalledWith({
      query: 'motherfucking search',
      order: MessageSearchOrder.RELEVANCE
    });
  });

  it('switches form state when SvelteKit reuses the page for another server', async () => {
    mocks.serverStores = {
      origin: serverStore('private origin query', MessageSearchOrder.NEWEST),
      remote: serverStore('remote query', MessageSearchOrder.RELEVANCE)
    };
    const { container } = render(SearchPage);
    const input = container.querySelector('input') as HTMLInputElement;
    expect(input.value).toBe('private origin query');

    activeServerId = 'remote';
    await tick();

    expect(input.value).toBe('remote query');
    await userEvent.click(
      [...container.querySelectorAll('button')].find((button) => button.textContent?.trim() === 'Search')!
    );
    expect(mocks.search).toHaveBeenCalledWith({
      query: 'remote query',
      order: MessageSearchOrder.RELEVANCE
    });
  });

  it('continues pagination when a filtered page has no visible results', async () => {
    let intersectionCallback: ((entries: IntersectionObserverEntry[]) => void) | undefined;
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(callback: (entries: IntersectionObserverEntry[]) => void) {
          intersectionCallback = callback;
        }
        observe = vi.fn();
        disconnect = vi.fn();
      }
    );
    mocks.serverStores = {
      origin: serverStore('', MessageSearchOrder.RELEVANCE, {
        nextCursor: 'filtered-page-cursor',
        hasSearched: true
      }),
      remote: serverStore()
    };

    render(SearchPage);
    await vi.waitFor(() => expect(intersectionCallback).toBeTypeOf('function'));
    intersectionCallback!([{ isIntersecting: true } as IntersectionObserverEntry]);

    await vi.waitFor(() => expect(mocks.loadMore).toHaveBeenCalledOnce());
  });
});
