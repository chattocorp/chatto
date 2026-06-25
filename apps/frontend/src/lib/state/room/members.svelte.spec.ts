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

function pageResult(
  users: ReturnType<typeof user>[],
  hasMore = false,
  totalCount = users.length
): OperationResult {
  return {
    data: {
      room: {
        members: {
          users,
          totalCount,
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
  it('eagerly loads every room member page into the canonical member list', async () => {
    const store = createStore([
      pageResult([user('u1', 'alice')], true, 3),
      pageResult([user('u2', 'boris'), user('u3', 'cora')], false, 3)
    ]);

    store.setRoom('room-1');
    await store.loadInitial();

    expect(store.members.map((member) => member.login)).toEqual(['alice', 'boris', 'cora']);
    expect(store.filteredMembers.map((member) => member.login)).toEqual(['alice', 'boris', 'cora']);
    expect(store.totalCount).toBe(3);
    expect(store.hasLoaded).toBe(true);
  });

  it('filters loaded members locally without changing the canonical count', async () => {
    const store = createStore([
      pageResult([user('u1', 'alice'), user('u2', 'boris'), user('u3', 'cora')], false, 3)
    ]);

    store.setRoom('room-1');
    await store.loadInitial();
    await store.setSearch('bo');

    expect(store.filteredMembers.map((member) => member.login)).toEqual(['boris']);
    expect(store.members.map((member) => member.login)).toEqual(['alice', 'boris', 'cora']);
    expect(store.totalCount).toBe(3);
  });

  it('marks failed initial loads as loaded to avoid immediate ensureLoaded retries', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeClient = new FakeGqlClient([{ data: null, error: new Error('network failed') }]);
    const store = new RoomMembersStore(fakeClient as unknown as GraphQLClient);

    try {
      store.setRoom('room-1');
      store.ensureLoaded();

      await vi.waitFor(() => {
        expect(store.hasLoaded).toBe(true);
        expect(store.isInitialLoading).toBe(false);
      });

      store.ensureLoaded();

      expect(fakeClient.queryMock).toHaveBeenCalledTimes(1);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('does not expose partial members when a later eager page fails', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeClient = new FakeGqlClient([
      pageResult([user('u1', 'alice')], true, 3),
      { data: null, error: new Error('network failed') }
    ]);
    const store = new RoomMembersStore(fakeClient as unknown as GraphQLClient);

    try {
      store.setRoom('room-1');
      await store.loadInitial();

      expect(store.members).toEqual([]);
      expect(store.totalCount).toBe(0);
      expect(store.hasLoaded).toBe(true);
      expect(store.filteredMembers).toEqual([]);
      expect(await store.searchMembers('alice')).toEqual([]);
      expect(fakeClient.queryMock).toHaveBeenCalledTimes(2);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('refresh clears a stale initial loading state when it invalidates an initial load', async () => {
    const initial = deferred<OperationResult>();
    const refresh = deferred<OperationResult>();
    const store = createStore([initial.promise, refresh.promise]);

    store.setRoom('room-1');
    const initialLoad = store.loadInitial();
    expect(store.isInitialLoading).toBe(true);

    const refreshLoad = store.refresh();
    expect(store.isInitialLoading).toBe(false);

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

  it('refresh reloads all pages and preserves local search as display-only state', async () => {
    const store = createStore([
      pageResult([user('u1', 'initial')], false, 1),
      pageResult([user('u2', 'refresh-a')], true, 3),
      pageResult([user('u3', 'refresh-b'), user('u4', 'other')], false, 3)
    ]);

    store.setRoom('room-1');
    await store.loadInitial();
    await store.setSearch('refresh');
    await store.refresh();

    expect(store.members.map((member) => member.login)).toEqual([
      'refresh-a',
      'refresh-b',
      'other'
    ]);
    expect(store.filteredMembers.map((member) => member.login)).toEqual(['refresh-a', 'refresh-b']);
    expect(store.totalCount).toBe(3);
  });

  it('preserves the previous complete snapshot when refresh fails mid-load', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeClient = new FakeGqlClient([
      pageResult([user('u1', 'initial')], false, 1),
      pageResult([user('u2', 'refresh-a')], true, 3),
      { data: null, error: new Error('network failed') }
    ]);
    const store = new RoomMembersStore(fakeClient as unknown as GraphQLClient);

    try {
      store.setRoom('room-1');
      await store.loadInitial();
      await store.refresh();

      expect(store.members.map((member) => member.login)).toEqual(['initial']);
      expect(store.totalCount).toBe(1);
      expect(store.hasLoaded).toBe(true);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});
