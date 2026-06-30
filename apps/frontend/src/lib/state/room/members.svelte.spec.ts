import { describe, expect, it, vi } from 'vitest';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';
import { PresenceStatus } from '$lib/render/types';
import type { MemberDirectoryAPI, MemberDirectoryPage } from '@chatto/api-client/memberDirectory';
import { ROOM_MEMBERS_PAGE_SIZE, RoomMembersStore } from './members.svelte';

class FakeMemberDirectoryAPI {
  listRoomMembers: MemberDirectoryAPI['listRoomMembers'];
  listServerMembers: MemberDirectoryAPI['listServerMembers'];

  constructor(results: Array<MemberDirectoryPage | Promise<MemberDirectoryPage>>) {
    const queue = [...results];
    this.listRoomMembers = vi.fn(async () => {
      const result = queue.shift();
      if (!result) throw new Error('Unexpected room members query');
      return result;
    });
    this.listServerMembers = vi.fn();
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
  it('eagerly loads every room member page into the canonical member list', async () => {
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 3),
      pageResult([user('u2', 'boris'), user('u3', 'cora')], false, 3)
    ]);
    const store = new RoomMembersStore(fakeAPI);

    store.setRoom('room-1');
    await store.loadInitial();

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
    const fakeAPI = new FakeMemberDirectoryAPI([Promise.reject(new Error('network failed'))]);
    const store = new RoomMembersStore(fakeAPI);

    try {
      store.setRoom('room-1');
      store.ensureLoaded();

      await vi.waitFor(() => {
        expect(store.hasLoaded).toBe(true);
        expect(store.isInitialLoading).toBe(false);
      });

      store.ensureLoaded();

      expect(fakeAPI.listRoomMembers).toHaveBeenCalledTimes(1);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('does not expose partial members when a later eager page fails', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fakeAPI = new FakeMemberDirectoryAPI([
      pageResult([user('u1', 'alice')], true, 3),
      Promise.reject(new Error('network failed'))
    ]);
    const store = new RoomMembersStore(fakeAPI);

    try {
      store.setRoom('room-1');
      await store.loadInitial();

      expect(store.members).toEqual([]);
      expect(store.totalCount).toBe(0);
      expect(store.hasLoaded).toBe(true);
      expect(store.filteredMembers).toEqual([]);
      expect(await store.searchMembers('alice')).toEqual([]);
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

  it('preserves the previous complete snapshot when refresh fails mid-load', async () => {
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

      expect(store.members.map((member) => member.login)).toEqual(['initial']);
      expect(store.totalCount).toBe(1);
      expect(store.hasLoaded).toBe(true);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});
