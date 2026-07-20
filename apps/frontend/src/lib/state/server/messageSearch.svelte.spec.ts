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
    expect(store.query).toBe('hello');
    expect(store.order).toBe(MessageSearchOrder.RELEVANCE);
    expect(store.hasSearched).toBe(true);
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

  it('retains empty-search state for browser Back restoration', async () => {
    const store = new MessageSearchStore(
      api({ searchMessages: vi.fn().mockResolvedValue(page([], null)) })
    );

    await store.search({ query: 'nothing', roomIds: [], order: MessageSearchOrder.NEWEST });

    expect(store.hasSearched).toBe(true);
    expect(store.query).toBe('nothing');
    expect(store.order).toBe(MessageSearchOrder.NEWEST);
    expect(store.results).toEqual([]);
  });

  it('clears plaintext results and invalidates in-flight work', async () => {
    let resolveSearch!: (value: MessageSearchPage) => void;
    const pending = new Promise<MessageSearchPage>((resolve) => (resolveSearch = resolve));
    const store = new MessageSearchStore(
      api({ searchMessages: vi.fn().mockReturnValue(pending) })
    );
    const search = store.search({
      query: 'hello',
      roomIds: [],
      order: MessageSearchOrder.RELEVANCE
    });

    store.clearResults();
    resolveSearch(page([result('stale')], 'stale-cursor'));
    await search;

    expect(store.results).toEqual([]);
    expect(store.nextCursor).toBeNull();
    expect(store.loading).toBe(false);
    expect(store.query).toBe('');
    expect(store.order).toBe(MessageSearchOrder.RELEVANCE);
    expect(store.hasSearched).toBe(false);
  });

  it('restarts after room revocation without destroying the search session', async () => {
    const searchMessages = vi
      .fn()
      .mockResolvedValueOnce(
        page([result('one'), { ...result('two'), roomId: 'room-2' }], 'next')
      )
      .mockResolvedValueOnce(page([{ ...result('two'), roomId: 'room-2' }], null))
      .mockResolvedValueOnce(page([], null));
    const store = new MessageSearchStore(
      api({ searchMessages })
    );
    await store.search({ query: 'hello', roomIds: [], order: MessageSearchOrder.NEWEST });

    store.revokeRoom('room-1');

    await vi.waitFor(() => expect(store.results.map((item) => item.id)).toEqual(['two']));
    expect(store.query).toBe('hello');
    expect(store.order).toBe(MessageSearchOrder.NEWEST);
    expect(store.hasSearched).toBe(true);
    expect(store.nextCursor).toBeNull();

    store.invalidateAuthor('user-1');
    await vi.waitFor(() => expect(store.results).toEqual([]));
    expect(store.query).toBe('hello');
    expect(searchMessages).toHaveBeenCalledTimes(3);
  });

  it('does not cancel an in-flight search for an unrelated new message', async () => {
    let resolveSearch!: (value: MessageSearchPage) => void;
    const pending = new Promise<MessageSearchPage>((resolve) => (resolveSearch = resolve));
    const store = new MessageSearchStore(
      api({ searchMessages: vi.fn().mockReturnValue(pending) })
    );

    const search = store.search({
      query: 'hello',
      roomIds: [],
      order: MessageSearchOrder.RELEVANCE
    });
    store.invalidateMessage('room-1', 'new-message');
    resolveSearch(page([result('one')], null));
    await search;

    expect(store.results.map((item) => item.id)).toEqual(['one']);
    expect(store.error).toBe(false);
  });
});
