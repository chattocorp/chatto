import { beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import type { MessageSearchResult } from '$lib/api-client/messageSearch';
import { RoomKind } from '$lib/api-client/roomDirectory';
import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
import SearchPageTestHarness from './SearchPageTestHarness.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    ensureStatus: vi.fn(),
    search: vi.fn(),
    clearResults: vi.fn(),
    prepareQueryChange: vi.fn(),
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
vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    Object.entries(params ?? {}).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));
vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => serverId,
  segmentToServerId: (serverId: string) => serverId
}));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: mocks.activeServer }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: (serverId: string) => mocks.serverStores[serverId],
    tryGetStore: (serverId: string) => mocks.serverStores[serverId]
  }
}));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    get serverId() {
      return activeServerId;
    },
    get store() {
      return mocks.serverStores[activeServerId];
    },
    connection: {},
    isCurrent: () => true
  })
}));

let activeServerId = $state('origin');

function serverStore(
  query = '',
  order = MessageSearchOrder.RELEVANCE,
  options: {
    nextCursor?: string | null;
    hasSearched?: boolean;
    results?: MessageSearchResult[];
  } = {}
) {
  const messageSearch = $state({
    status: { state: MessageSearchState.READY, retryAfterMs: null },
    statusLoading: false,
    statusLoaded: true,
    statusError: false,
    available: true,
    query,
    order,
    results: options.results ?? [],
    nextCursor: options.nextCursor ?? null,
    loading: false,
    loadingMore: false,
    error: false,
    hasSearched: options.hasSearched ?? false,
    ensureStatus: mocks.ensureStatus,
    refreshStatus: vi.fn(),
    search: mocks.search,
    clearResults: mocks.clearResults,
    prepareQueryChange: mocks.prepareQueryChange,
    loadMore: mocks.loadMore
  });
  return {
    currentUser: { user: { settings: null } },
    messageSearch
  };
}

function fillSearchInput(input: HTMLInputElement, value: string): void {
  input.value = value;
  input.dispatchEvent(
    new InputEvent('input', { bubbles: true, inputType: 'insertText', data: value })
  );
}

describe('message search page', () => {
  const waitForSearchDebounce = () => new Promise((resolve) => setTimeout(resolve, 350));

  beforeEach(() => {
    vi.clearAllMocks();
    activeServerId = 'origin';
    mocks.activeServer.mockImplementation(() => activeServerId);
    mocks.serverStores = { origin: serverStore(), remote: serverStore() };
  });

  it('mounts as a server page and debounces unscoped searches without a button', async () => {
    const { container } = render(SearchPageTestHarness);

    const input = container.querySelector('input') as HTMLInputElement;
    fillSearchInput(input, 'motherfucking search');
    expect(mocks.search).not.toHaveBeenCalled();
    await waitForSearchDebounce();

    expect(container.textContent).toContain('Search messages');
    expect(
      [...container.querySelectorAll('h2')].map((heading) => heading.textContent?.trim())
    ).toEqual(['Search query', 'Results']);
    expect(container.textContent).not.toContain('All rooms');
    expect(
      [...container.querySelectorAll('button')].some(
        (button) => button.textContent?.trim() === 'Search'
      )
    ).toBe(false);
    expect(mocks.ensureStatus).toHaveBeenCalledOnce();
    expect(mocks.search).toHaveBeenCalledWith(
      {
        query: 'motherfucking search',
        order: MessageSearchOrder.RELEVANCE
      },
      { preserveQuery: true }
    );
    expect(document.activeElement).toBe(input);

    fillSearchInput(input, 'replacement query');
    input.form!.requestSubmit();

    expect(mocks.search).toHaveBeenLastCalledWith(
      {
        query: 'replacement query',
        order: MessageSearchOrder.RELEVANCE
      },
      { preserveQuery: true }
    );
    await waitForSearchDebounce();
    expect(mocks.search).toHaveBeenCalledTimes(2);
  });

  it('does not repeat a completed search for trailing whitespace', async () => {
    const { container } = render(SearchPageTestHarness);
    const input = container.querySelector('input') as HTMLInputElement;
    const store = mocks.serverStores.origin as ReturnType<typeof serverStore>;

    fillSearchInput(input, 'foo');
    await waitForSearchDebounce();
    store.messageSearch.hasSearched = true;
    fillSearchInput(input, 'foo ');
    await waitForSearchDebounce();

    expect(input.value).toBe('foo ');
    expect(mocks.search).toHaveBeenCalledOnce();
    expect(mocks.search).toHaveBeenCalledWith(
      { query: 'foo', order: MessageSearchOrder.RELEVANCE },
      { preserveQuery: true }
    );
  });

  it('switches form state when SvelteKit reuses the page for another server', async () => {
    mocks.serverStores = {
      origin: serverStore('private origin query', MessageSearchOrder.NEWEST),
      remote: serverStore('remote query', MessageSearchOrder.RELEVANCE)
    };
    const { container } = render(SearchPageTestHarness);
    const input = container.querySelector('input') as HTMLInputElement;
    expect(input.value).toBe('private origin query');

    activeServerId = 'remote';
    await tick();

    expect(input.value).toBe('remote query');
    await userEvent.keyboard('{Enter}');
    expect(mocks.search).toHaveBeenCalledWith(
      { query: 'remote query', order: MessageSearchOrder.RELEVANCE },
      { preserveQuery: true }
    );
  });

  it('reruns a completed search immediately when its order changes', async () => {
    mocks.serverStores = {
      origin: serverStore('needle', MessageSearchOrder.RELEVANCE, { hasSearched: true }),
      remote: serverStore()
    };
    const { container } = render(SearchPageTestHarness);

    await userEvent.click(
      [...container.querySelectorAll('label')].find(
        (label) => label.textContent?.trim() === 'Newest'
      )!
    );

    expect(mocks.search).toHaveBeenCalledWith(
      { query: 'needle', order: MessageSearchOrder.NEWEST },
      { preserveQuery: true }
    );
  });

  it('uses a new order immediately when a search is still pending', async () => {
    const { container } = render(SearchPageTestHarness);
    const input = container.querySelector('input') as HTMLInputElement;

    fillSearchInput(input, 'needle');
    [...container.querySelectorAll('label')]
      .find((label) => label.textContent?.trim() === 'Newest')!
      .click();

    expect(mocks.search).toHaveBeenCalledOnce();
    expect(mocks.search).toHaveBeenCalledWith(
      { query: 'needle', order: MessageSearchOrder.NEWEST },
      { preserveQuery: true }
    );
    await waitForSearchDebounce();
    expect(mocks.search).toHaveBeenCalledOnce();
  });

  it('discards a pending search when the store is externally cleared', async () => {
    const { container } = render(SearchPageTestHarness);
    const input = container.querySelector('input') as HTMLInputElement;
    const store = mocks.serverStores.origin as ReturnType<typeof serverStore>;

    fillSearchInput(input, 'private query');
    store.messageSearch.query = '';
    store.messageSearch.hasSearched = false;
    await tick();
    await waitForSearchDebounce();

    expect(input.value).toBe('');
    expect(mocks.search).not.toHaveBeenCalled();
  });

  it('allows Enter to retry a failed search', async () => {
    const { container } = render(SearchPageTestHarness);
    const input = container.querySelector('input') as HTMLInputElement;
    const store = mocks.serverStores.origin as ReturnType<typeof serverStore>;

    fillSearchInput(input, 'retry me');
    await waitForSearchDebounce();
    store.messageSearch.hasSearched = true;
    store.messageSearch.error = true;
    input.form!.requestSubmit();

    expect(mocks.search).toHaveBeenCalledTimes(2);
    expect(mocks.search).toHaveBeenLastCalledWith(
      { query: 'retry me', order: MessageSearchOrder.RELEVANCE },
      { preserveQuery: true }
    );
  });

  it('allows Enter to refresh a completed search', async () => {
    const { container } = render(SearchPageTestHarness);
    const input = container.querySelector('input') as HTMLInputElement;
    const store = mocks.serverStores.origin as ReturnType<typeof serverStore>;

    fillSearchInput(input, 'refresh me');
    await waitForSearchDebounce();
    store.messageSearch.hasSearched = true;
    input.form!.requestSubmit();

    expect(mocks.search).toHaveBeenCalledTimes(2);
    expect(mocks.search).toHaveBeenLastCalledWith(
      { query: 'refresh me', order: MessageSearchOrder.RELEVANCE },
      { preserveQuery: true }
    );
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

    render(SearchPageTestHarness);
    await vi.waitFor(() => expect(intersectionCallback).toBeTypeOf('function'));
    intersectionCallback!([{ isIntersecting: true } as IntersectionObserverEntry]);

    await vi.waitFor(() => expect(mocks.loadMore).toHaveBeenCalledOnce());
  });

  it('renders rich message results with room and thread-aware message links', async () => {
    mocks.serverStores = {
      origin: serverStore('', MessageSearchOrder.RELEVANCE, {
        hasSearched: true,
        results: [
          {
            id: 'message-1',
            roomId: 'room-1',
            roomName: 'general',
            roomKind: RoomKind.CHANNEL,
            actorId: 'user-1',
            actor: {
              id: 'user-1',
              login: 'alice',
              displayName: 'Alice',
              deleted: false,
              avatarUrl: null
            },
            body: 'A **searchable** [message](https://example.com)',
            createdAt: '2026-07-22T09:42:00.000Z',
            threadRootEventId: 'thread-root',
            attachmentCount: 2,
            relevanceScore: 8.5
          },
          {
            id: 'message-2',
            roomId: 'dm-1',
            roomName: '',
            roomKind: RoomKind.DM,
            actorId: 'user-unavailable',
            actor: null,
            body: 'Message with unavailable actor hydration',
            createdAt: '2026-07-21T09:42:00.000Z',
            threadRootEventId: null,
            attachmentCount: 0,
            relevanceScore: 3.25
          }
        ]
      }),
      remote: serverStore()
    };

    const { container } = render(SearchPageTestHarness);
    await vi.waitFor(() =>
      expect(container.querySelector('[role="article"] strong')?.textContent).toContain('Alice')
    );

    await vi.waitFor(() =>
      expect(container.querySelector('[role="article"] .prose strong')?.textContent).toBe(
        'searchable'
      )
    );
    expect(container.querySelector('a[href="/chat/origin/room-1"]')?.textContent?.trim()).toBe(
      '#general'
    );
    expect(container.querySelector('a[href="/chat/origin/dm-1"]')?.textContent?.trim()).toBe(
      'Direct Message'
    );
    expect(
      container
        .querySelector('a[href="/chat/origin/room-1/thread-root/m/message-1"] time')
        ?.getAttribute('datetime')
    ).toBe('2026-07-22T09:42:00.000Z');
    expect(container.querySelector('[role="article"]')?.textContent).toContain('2');
    expect(container.querySelector('[role="article"] .uil--paperclip')).not.toBeNull();
    expect(container.querySelector('[role="article"] button')).toBeNull();
    expect(container.querySelectorAll('[role="article"]')[1]?.textContent).toContain('Unknown');
    expect(container.querySelectorAll('[role="article"]')[1]?.textContent).not.toContain(
      'Deleted user'
    );

    const firstResult = container.querySelector(
      '[data-search-result-id="message-1"]'
    ) as HTMLElement;
    expect(firstResult.getAttribute('role')).toBe('link');
    expect(firstResult.querySelector('.message-row')?.classList).toContain('md:mx-0');
    expect(container.querySelector('ol')?.classList).not.toContain('divide-y');
    expect(container.querySelector('ol')?.classList).toContain('gap-4');

    await userEvent.click(firstResult);
    expect(mocks.goto).toHaveBeenCalledWith('/chat/origin/room-1/thread-root/m/message-1');

    mocks.goto.mockClear();
    await userEvent.click(firstResult.querySelector('.prose a')!);
    expect(mocks.goto).toHaveBeenCalledOnce();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/origin/room-1/thread-root/m/message-1');
  });
});
