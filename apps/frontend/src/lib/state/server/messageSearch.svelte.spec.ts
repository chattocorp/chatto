import { describe, expect, it, vi } from 'vitest';
import type {
  MessageSearchAPI,
  MessageSearchPage,
  MessageSearchResult,
  MessageSearchStatus
} from '$lib/api-client/messageSearch';
import { RoomKind } from '$lib/api-client/roomDirectory';
import { MessageSearchOrder, MessageSearchState, MessageSearchStore } from './messageSearch.svelte';

function result(id: string): MessageSearchResult {
  return {
    id,
    roomId: 'room-1',
    roomName: 'general',
    roomKind: RoomKind.CHANNEL,
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
      roomId: 'room-1',
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
      order: MessageSearchOrder.RELEVANCE
    });
    await store.search({ query: 'new', order: MessageSearchOrder.NEWEST });
    resolveFirst(page([result('old')], null));
    await older;

    expect(store.results.map((item) => item.id)).toEqual(['new']);
  });

  it('allows pagination after a new search supersedes an in-flight page', async () => {
    let resolveStalePage!: (value: MessageSearchPage) => void;
    const stalePage = new Promise<MessageSearchPage>((resolve) => (resolveStalePage = resolve));
    const searchMessages = vi
      .fn()
      .mockResolvedValueOnce(page([result('old')], 'old-cursor'))
      .mockReturnValueOnce(stalePage)
      .mockResolvedValueOnce(page([result('new')], 'new-cursor'))
      .mockResolvedValueOnce(page([result('newer')], null));
    const store = new MessageSearchStore(api({ searchMessages }));

    await store.search({ query: 'old', order: MessageSearchOrder.RELEVANCE });
    const staleLoadMore = store.loadMore();
    expect(store.loadingMore).toBe(true);

    await store.search({ query: 'new', order: MessageSearchOrder.NEWEST });
    expect(store.loadingMore).toBe(false);
    resolveStalePage(page([result('stale')], null));
    await staleLoadMore;
    await store.loadMore();

    expect(searchMessages).toHaveBeenCalledTimes(4);
    expect(store.results.map((item) => item.id)).toEqual(['new', 'newer']);
    expect(store.nextCursor).toBeNull();
  });

  it('fences status responses and cleanup across reset', async () => {
    let resolveStaleStatus!: (value: MessageSearchStatus) => void;
    let resolveCurrentStatus!: (value: MessageSearchStatus) => void;
    const staleStatus = new Promise<MessageSearchStatus>(
      (resolve) => (resolveStaleStatus = resolve)
    );
    const currentStatus = new Promise<MessageSearchStatus>(
      (resolve) => (resolveCurrentStatus = resolve)
    );
    const store = new MessageSearchStore(
      api({
        getStatus: vi.fn().mockReturnValueOnce(staleStatus).mockReturnValueOnce(currentStatus)
      })
    );

    const staleRequest = store.ensureStatus();
    store.reset();
    const currentRequest = store.ensureStatus();
    resolveStaleStatus({ state: MessageSearchState.READY, retryAfterMs: null });
    await staleRequest;

    expect(store.status).toEqual({
      state: MessageSearchState.UNSPECIFIED,
      retryAfterMs: null
    });
    expect(store.statusLoaded).toBe(false);
    expect(store.statusLoading).toBe(true);

    resolveCurrentStatus({ state: MessageSearchState.DEGRADED, retryAfterMs: 1000 });
    await currentRequest;

    expect(store.status).toEqual({
      state: MessageSearchState.DEGRADED,
      retryAfterMs: 1000
    });
    expect(store.statusLoaded).toBe(true);
    expect(store.statusLoading).toBe(false);
  });

  it('retains empty-search state for browser Back restoration', async () => {
    const store = new MessageSearchStore(
      api({ searchMessages: vi.fn().mockResolvedValue(page([], null)) })
    );

    await store.search({ query: 'nothing', order: MessageSearchOrder.NEWEST });

    expect(store.hasSearched).toBe(true);
    expect(store.query).toBe('nothing');
    expect(store.order).toBe(MessageSearchOrder.NEWEST);
    expect(store.results).toEqual([]);
  });

  it('clears plaintext results and invalidates in-flight work', async () => {
    let resolveSearch!: (value: MessageSearchPage) => void;
    const pending = new Promise<MessageSearchPage>((resolve) => (resolveSearch = resolve));
    const store = new MessageSearchStore(api({ searchMessages: vi.fn().mockReturnValue(pending) }));
    const search = store.search({
      query: 'hello',
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
      .mockResolvedValueOnce(page([result('one'), { ...result('two'), roomId: 'room-2' }], 'next'))
      .mockResolvedValueOnce(page([{ ...result('two'), roomId: 'room-2' }], null))
      .mockResolvedValueOnce(page([], null));
    const store = new MessageSearchStore(api({ searchMessages }));
    await store.search({ query: 'hello', order: MessageSearchOrder.NEWEST });

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
    const store = new MessageSearchStore(api({ searchMessages: vi.fn().mockReturnValue(pending) }));

    const search = store.search({
      query: 'hello',
      order: MessageSearchOrder.RELEVANCE
    });
    store.invalidateMessage('room-1', 'new-message');
    resolveSearch(page([result('one')], null));
    await search;

    expect(store.results.map((item) => item.id)).toEqual(['one']);
    expect(store.error).toBe(false);
  });

  it('fences an in-flight search on a content-free realtime refresh', async () => {
    let resolveFirst!: (value: MessageSearchPage) => void;
    const firstPage = new Promise<MessageSearchPage>((resolve) => (resolveFirst = resolve));
    const searchMessages = vi
      .fn()
      .mockReturnValueOnce(firstPage)
      .mockResolvedValueOnce(page([result('fresh')], null));
    const store = new MessageSearchStore(api({ searchMessages }));

    const firstSearch = store.search({
      query: 'hello',
      order: MessageSearchOrder.RELEVANCE
    });
    store.refreshRetainedResults();

    await vi.waitFor(() => expect(store.results.map((item) => item.id)).toEqual(['fresh']));
    resolveFirst(page([result('stale')], null));
    await firstSearch;

    expect(store.results.map((item) => item.id)).toEqual(['fresh']);
    expect(searchMessages).toHaveBeenCalledTimes(2);
  });
});
