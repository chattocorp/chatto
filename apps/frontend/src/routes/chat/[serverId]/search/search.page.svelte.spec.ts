import { beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
import SearchPage from './+page.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    ensureStatus: vi.fn(),
    search: vi.fn(),
    loadMore: vi.fn(),
    goto: vi.fn()
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn()
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => 'origin' }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      currentUser: { user: { settings: null } },
      messageSearch: {
        status: {
          state: MessageSearchState.READY,
          indexedEventCount: null,
          targetEventCount: null
        },
        statusLoading: false,
        statusLoaded: true,
        statusError: false,
        available: true,
        query: '',
        order: MessageSearchOrder.RELEVANCE,
        results: [],
        nextCursor: null,
        loading: false,
        loadingMore: false,
        error: false,
        hasSearched: false,
        ensureStatus: mocks.ensureStatus,
        refreshStatus: vi.fn(),
        search: mocks.search,
        loadMore: mocks.loadMore
      }
    })
  }
}));

describe('message search page', () => {
  beforeEach(() => vi.clearAllMocks());

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
      roomIds: [],
      order: MessageSearchOrder.RELEVANCE
    });
  });
});
