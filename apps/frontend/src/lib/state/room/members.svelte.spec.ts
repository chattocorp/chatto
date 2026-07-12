import { describe, expect, it, vi } from 'vitest';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';
import { PresenceStatus } from '$lib/render/types';
import type { MemberDirectoryAPI, MemberDirectoryPage } from '$lib/api-client/memberDirectory';
import { ROOM_MEMBERS_PAGE_SIZE, RoomMembersStore } from './members.svelte';

class FakeMemberDirectoryAPI {
  listRoomMembers: MemberDirectoryAPI['listRoomMembers'];
  listUsers: MemberDirectoryAPI['listUsers'];
  getUser: MemberDirectoryAPI['getUser'];
  getUserByLogin: MemberDirectoryAPI['getUserByLogin'];
  batchGetUsers: MemberDirectoryAPI['batchGetUsers'];
  getRoomMember: MemberDirectoryAPI['getRoomMember'];
  batchGetRoomMembers: MemberDirectoryAPI['batchGetRoomMembers'];

  constructor(results: Array<MemberDirectoryPage | Promise<MemberDirectoryPage>>) {
    const queue = [...results];
    this.listRoomMembers = vi.fn(async () => {
      const result = queue.shift();
      if (!result) throw new Error('Unexpected room members query');
      return result;
    });
    this.listUsers = vi.fn();
    this.getUser = vi.fn();
    this.getUserByLogin = vi.fn();
    this.batchGetUsers = vi.fn();
    this.getRoomMember = vi.fn();
    this.batchGetRoomMembers = vi.fn();
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
    id,
    login,
    displayName: login,
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online,
    customStatus: null,
    roles: [],
    createdAt: null
  };
}

function pageResult(
  users: ReturnType<typeof user>[],
  hasMore = false,
  totalCount = users.length
): MemberDirectoryPage {
  return {
    members: users,
    totalCount,
    hasMore
  };
}

function createStore(results: Array<MemberDirectoryPage | Promise<MemberDirectoryPage>>) {
  return new RoomMembersStore(new FakeMemberDirectoryAPI(results));
}

describe('RoomMembersStore', () => {
  it('requests room members in 250-member pages', () => {
    expect(ROOM_MEMBERS_PAGE_SIZE).toBe(250);
  });

  it('publishes the first page before hydrating the canonical member list in the background', async () => {
    const backgroundPage = deferred<MemberDirectoryPage>();
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 3),
      backgroundPage.promise
    ]);
    const store = new RoomMembersStore(fakeAPI);

    store.setRoom('room-1');
    const loading = store.loadInitial();

    await vi.waitFor(() => {
      expect(store.hasFirstPage).toBe(true);
      expect(store.isInitialLoading).toBe(false);
      expect(store.isBackgroundLoading).toBe(true);
      expect(store.members.map((member) => member.login)).toEqual(['alice']);
    });

    backgroundPage.resolve(pageResult([user('u2', 'boris'), user('u3', 'cora')], false, 3));
    await loading;

    expect(fakeAPI.listRoomMembers).toHaveBeenNthCalledWith(
      1,
      'room-1',
      '',
      ROOM_MEMBERS_PAGE_SIZE,
      0
    );
    expect(fakeAPI.listRoomMembers).toHaveBeenNthCalledWith(
      2,
      'room-1',
      '',
      ROOM_MEMBERS_PAGE_SIZE,
      1
    );
    expect(store.members.map((member) => member.login)).toEqual(['alice', 'boris', 'cora']);
    expect(store.filteredMembers.map((member) => member.login)).toEqual(['alice', 'boris', 'cora']);
    expect(store.totalCount).toBe(3);
    expect(store.hasLoaded).toBe(true);
    expect(store.hasLoadedAll).toBe(true);
    expect(store.isBackgroundLoading).toBe(false);
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

  it('searches the server for an unhydrated member and merges the result', async () => {
    const backgroundPage = deferred<MemberDirectoryPage>();
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 3),
      backgroundPage.promise,
      pageResult([user('u3', 'cora')], false, 1)
    ]);
    const store = new RoomMembersStore(fakeAPI);

    store.setRoom('room-1');
    const loading = store.loadInitial();
    await vi.waitFor(() => expect(store.hasFirstPage).toBe(true));

    await store.setSearch('cor');
    expect(fakeAPI.listRoomMembers).toHaveBeenNthCalledWith(
      3,
      'room-1',
      'cor',
      ROOM_MEMBERS_PAGE_SIZE,
      0
    );
    expect(store.filteredMembers.map((member) => member.login)).toEqual(['cora']);
    expect(store.members.map((member) => member.login)).toEqual(['alice']);

    backgroundPage.resolve(pageResult([user('u2', 'boris'), user('u3', 'cora')], false, 3));
    await loading;

    expect(store.members.map((member) => member.login)).toEqual(['alice', 'boris', 'cora']);
    expect(new Set(store.members.map((member) => member.id)).size).toBe(3);
  });

  it('discards an in-flight search when a same-room refresh starts', async () => {
    const backgroundPage = deferred<MemberDirectoryPage>();
    const staleSearch = deferred<MemberDirectoryPage>();
    const refreshedPage = deferred<MemberDirectoryPage>();
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 2),
      backgroundPage.promise,
      staleSearch.promise,
      refreshedPage.promise
    ]);
    const store = new RoomMembersStore(fakeAPI);

    store.setRoom('room-1');
    const initialLoad = store.loadInitial();
    await vi.waitFor(() => expect(store.hasFirstPage).toBe(true));

    const searching = store.searchMembers('departed');
    const refreshing = store.refresh();
    refreshedPage.resolve(pageResult([user('u1', 'alice')], false, 1));
    await refreshing;

    staleSearch.resolve(pageResult([user('u2', 'departed')], false, 1));
    await expect(searching).resolves.toEqual([]);
    expect(store.members.map((member) => member.login)).toEqual(['alice']);

    backgroundPage.resolve(pageResult([user('u2', 'departed')], false, 2));
    await initialLoad;
    expect(store.members.map((member) => member.login)).toEqual(['alice']);
  });

  it('pages through every sidebar search match while hydration is incomplete', async () => {
    const backgroundPage = deferred<MemberDirectoryPage>();
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 3),
      backgroundPage.promise,
      pageResult([user('u2', 'match-a')], true, 2),
      pageResult([user('u3', 'match-b')], false, 2)
    ]);
    const store = new RoomMembersStore(fakeAPI);

    store.setRoom('room-1');
    const loading = store.loadInitial();
    await vi.waitFor(() => expect(store.hasFirstPage).toBe(true));

    await store.setSearch('match');

    expect(fakeAPI.listRoomMembers).toHaveBeenNthCalledWith(
      3,
      'room-1',
      'match',
      ROOM_MEMBERS_PAGE_SIZE,
      0
    );
    expect(fakeAPI.listRoomMembers).toHaveBeenNthCalledWith(
      4,
      'room-1',
      'match',
      ROOM_MEMBERS_PAGE_SIZE,
      1
    );
    expect(store.filteredMembers.map((member) => member.login)).toEqual(['match-a', 'match-b']);

    backgroundPage.resolve(pageResult([user('u2', 'match-a'), user('u3', 'match-b')], false, 3));
    await loading;
  });

  it('records failed initial loads to avoid immediate ensureLoaded retries', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeAPI = new FakeMemberDirectoryAPI([Promise.reject(new Error('network failed'))]);
    const store = new RoomMembersStore(fakeAPI);

    try {
      store.setRoom('room-1');
      store.ensureLoaded();

      await vi.waitFor(() => {
        expect(store.loadError).toBe('network failed');
        expect(store.isInitialLoading).toBe(false);
      });

      store.ensureLoaded();

      expect(fakeAPI.listRoomMembers).toHaveBeenCalledTimes(1);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('keeps the published first page when background hydration fails', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 3),
      Promise.reject(new Error('network failed'))
    ]);
    const store = new RoomMembersStore(fakeAPI);

    try {
      store.setRoom('room-1');
      await store.loadInitial();

      expect(store.members.map((member) => member.login)).toEqual(['alice']);
      expect(store.totalCount).toBe(3);
      expect(store.hasFirstPage).toBe(true);
      expect(store.hasLoadedAll).toBe(false);
      expect(store.loadError).toBe('network failed');
      expect(fakeAPI.listRoomMembers).toHaveBeenCalledTimes(2);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('refresh clears a stale initial loading state when it invalidates an initial load', async () => {
    const initial = deferred<MemberDirectoryPage>();
    const refresh = deferred<MemberDirectoryPage>();
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

  it('refreshes from room membership events using local event kind', async () => {
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'initial')], false, 1),
      pageResult([user('u2', 'joined')], false, 1)
    ]);
    const store = new RoomMembersStore(fakeAPI);

    store.setRoom('room-1');
    await store.loadInitial();

    store.ingestServerEvent({
      id: 'evt-1',
      roomId: 'room-1',
      actorId: 'u2',
      createdAt: new Date().toISOString(),
      event: {
        kind: RoomEventKind.UserJoinedRoom,
        roomId: 'room-1'
      }
    } as EventEnvelope);

    await vi.waitFor(() => {
      expect(fakeAPI.listRoomMembers).toHaveBeenCalledTimes(2);
      expect(store.members.map((member) => member.login)).toEqual(['joined']);
    });
  });

  it('publishes a refreshed first page when later refresh hydration fails', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'initial')], false, 1),
      pageResult([user('u2', 'refresh-a')], true, 3),
      Promise.reject(new Error('network failed'))
    ]);
    const store = new RoomMembersStore(fakeAPI);

    try {
      store.setRoom('room-1');
      await store.loadInitial();
      await store.refresh();

      expect(store.members.map((member) => member.login)).toEqual(['refresh-a']);
      expect(store.totalCount).toBe(3);
      expect(store.hasLoadedAll).toBe(false);
      expect(store.loadError).toBe('network failed');
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});
