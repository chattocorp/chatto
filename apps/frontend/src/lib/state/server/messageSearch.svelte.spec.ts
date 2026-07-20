import { describe, expect, it, vi } from 'vitest';
import type {
  MessageSearchAPI,
  MessageSearchPage,
  MessageSearchResult
} from '$lib/api-client/messageSearch';
import { MessageSearchOrder, MessageSearchState, MessageSearchStore } from './messageSearch.svelte';

function result(id: string): MessageSearchResult {
  return {
    id,
    roomId: 'room-1',
    roomName: 'general',
    actorId: 'user-1',
    actor: null,
    body: `message ${id}`,
    createdAt: '2026-01-01T12:00:00.000Z',
    threadRootEventId: null,
    attachmentCount: 0
  };
}

function page(results: MessageSearchResult[], nextCursor: string | null): MessageSearchPage {
  return { results, nextCursor };
}

function api(overrides: Partial<MessageSearchAPI> = {}): MessageSearchAPI {
  return {
    getStatus: vi.fn().mockResolvedValue({
      state: MessageSearchState.READY,
      indexedEventCount: null,
      targetEventCount: null,
      retryAfterMs: null
    }),
    searchMessages: vi.fn().mockResolvedValue(page([result('one')], null)),
    ...overrides
  };
}

describe('MessageSearchStore', () => {
  it('loads availability only once', async () => {
    const client = api();
    const store = new MessageSearchStore(client);

    await Promise.all([store.ensureStatus(), store.ensureStatus()]);

    expect(client.getStatus).toHaveBeenCalledOnce();
    expect(store.available).toBe(true);
  });

  it('searches and automatically appends deduplicated cursor pages', async () => {
    const client = api({
      searchMessages: vi
        .fn()
        .mockResolvedValueOnce(page([result('one')], 'next'))
        .mockResolvedValueOnce(page([result('one'), result('two')], null))
    });
    const store = new MessageSearchStore(client);
    const input = {
      query: 'hello',
      roomIds: ['room-1'],
      order: MessageSearchOrder.RELEVANCE
    };

    await store.search(input);
    await store.loadMore();

    expect(client.searchMessages).toHaveBeenNthCalledWith(1, input);
    expect(client.searchMessages).toHaveBeenNthCalledWith(2, { ...input, cursor: 'next' });
    expect(store.results.map((item) => item.id)).toEqual(['one', 'two']);
    expect(store.nextCursor).toBeNull();
  });

  it('ignores an older response after a newer query starts', async () => {
    let resolveFirst!: (value: MessageSearchPage) => void;
    const first = new Promise<MessageSearchPage>((resolve) => (resolveFirst = resolve));
    const client = api({
      searchMessages: vi
        .fn()
        .mockReturnValueOnce(first)
        .mockResolvedValueOnce(page([result('new')], null))
    });
    const store = new MessageSearchStore(client);

    const older = store.search({
      query: 'old',
      roomIds: [],
      order: MessageSearchOrder.RELEVANCE
    });
    await store.search({ query: 'new', roomIds: [], order: MessageSearchOrder.NEWEST });
    resolveFirst(page([result('old')], null));
    await older;

    expect(store.results.map((item) => item.id)).toEqual(['new']);
  });

  it('clears plaintext results and invalidates in-flight work', async () => {
    const store = new MessageSearchStore(api());
    await store.search({ query: 'hello', roomIds: [], order: MessageSearchOrder.RELEVANCE });

    store.clearResults();

    expect(store.results).toEqual([]);
    expect(store.nextCursor).toBeNull();
    expect(store.loading).toBe(false);
  });
});
