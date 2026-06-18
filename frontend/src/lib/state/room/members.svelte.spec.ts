import { describe, expect, it, vi } from 'vitest';
import type { Client } from '@urql/svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import { PresenceStatus } from '$lib/gql/graphql';
import { RoomMembersStore } from './members.svelte';

type OperationResult = {
  data?: unknown;
  error?: unknown;
};

class FakeGqlClient {
  client: Client;
  queryMock: ReturnType<typeof vi.fn>;

  constructor(results: Array<OperationResult | Promise<OperationResult>>) {
    const queue = [...results];
    this.queryMock = vi.fn(() => ({
      toPromise: async () => {
        const result = queue.shift();
        if (!result) throw new Error('Unexpected room members query');
        return result;
      }
    }));
    this.client = {
      query: this.queryMock,
      mutation: vi.fn(),
      subscription: vi.fn()
    } as unknown as Client;
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function user(id: string, login = id) {
  return {
    __typename: 'User',
    id,
    login,
    displayName: login,
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online
  };
}

function pageResult(users: ReturnType<typeof user>[], hasMore = false): OperationResult {
  return {
    data: {
      room: {
        members: {
          users,
          totalCount: users.length,
          hasMore
        }
      }
    },
    error: null
  };
}

function createStore(results: Array<OperationResult | Promise<OperationResult>>) {
  return new RoomMembersStore(new FakeGqlClient(results) as unknown as GraphQLClient);
}

describe('RoomMembersStore', () => {
  it('refresh clears a stale initial loading state when it invalidates an initial load', async () => {
    const initial = deferred<OperationResult>();
    const refresh = deferred<OperationResult>();
    const store = createStore([initial.promise, refresh.promise]);

    store.setRoom('room-1');
    const initialLoad = store.loadInitial();
    expect(store.isInitialLoading).toBe(true);

    const refreshLoad = store.refresh();
    expect(store.isInitialLoading).toBe(false);
    expect(store.isLoadingMore).toBe(false);

    refresh.resolve(pageResult([user('u2', 'refresh')]));
    await refreshLoad;

    expect(store.hasLoaded).toBe(true);
    expect(store.isInitialLoading).toBe(false);
    expect(store.members.map((member) => member.id)).toEqual(['u2']);

    initial.resolve(pageResult([user('u1', 'initial')]));
    await initialLoad;

    expect(store.isInitialLoading).toBe(false);
    expect(store.members.map((member) => member.id)).toEqual(['u2']);
  });

  it('refresh clears a stale load-more state when it invalidates pagination', async () => {
    const loadMore = deferred<OperationResult>();
    const refresh = deferred<OperationResult>();
    const store = createStore([
      pageResult([user('u1', 'initial')], true),
      loadMore.promise,
      refresh.promise
    ]);

    store.setRoom('room-1');
    await store.loadInitial();
    expect(store.hasMore).toBe(true);

    const loadMoreRequest = store.loadMore();
    expect(store.isLoadingMore).toBe(true);

    const refreshRequest = store.refresh();
    expect(store.isLoadingMore).toBe(false);

    refresh.resolve(pageResult([user('u3', 'refresh')]));
    await refreshRequest;

    expect(store.isLoadingMore).toBe(false);
    expect(store.members.map((member) => member.id)).toEqual(['u3']);

    loadMore.resolve(pageResult([user('u2', 'more')]));
    await loadMoreRequest;

    expect(store.isLoadingMore).toBe(false);
    expect(store.members.map((member) => member.id)).toEqual(['u3']);
  });
});
