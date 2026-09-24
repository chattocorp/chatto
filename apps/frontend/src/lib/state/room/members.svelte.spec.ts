import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { describe, expect, it, vi } from 'vitest';

import type { MemberDirectoryAPI, MemberDirectoryPage } from '$lib/api-client/memberDirectory';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { disposeUserStore, getUserStore } from '$lib/state/server/users.svelte';
import { ROOM_MEMBERS_PAGE_SIZE, RoomMembersStore } from './members.svelte';

class FakeMemberDirectoryAPI {
  listRoomMembers: MemberDirectoryAPI['listRoomMembers'];
  listOnlineRoomMembers: MemberDirectoryAPI['listOnlineRoomMembers'];
  listUsers: MemberDirectoryAPI['listUsers'];
  getUser: MemberDirectoryAPI['getUser'];
  getUserByLogin: MemberDirectoryAPI['getUserByLogin'];
  batchGetUsers: MemberDirectoryAPI['batchGetUsers'];
  getRoomMember: MemberDirectoryAPI['getRoomMember'];
  batchGetRoomMembers: MemberDirectoryAPI['batchGetRoomMembers'];

  constructor(results: Array<MemberDirectoryPage | Promise<MemberDirectoryPage>>) {
    const queue = [...results];
    this.listOnlineRoomMembers = vi.fn(async () => pageResult([]));
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

function user(id: string, login = id, isBot = false) {
  return {
    id,
    login,
    displayName: login,
    deleted: false,
    isBot,
    avatarUrl: null,
    presenceStatus: PresenceStatus.ONLINE,
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
  it('publishes connected names while the full page is pending and preserves them between full pages', async () => {
    const first = deferred<MemberDirectoryPage>();
    const last = deferred<MemberDirectoryPage>();
    const api = new FakeMemberDirectoryAPI([first.promise, last.promise]);
    api.listOnlineRoomMembers = vi.fn(async (_room, status) =>
      status === PresenceStatus.AWAY
        ? pageResult([{ ...user('online'), presenceStatus: PresenceStatus.OFFLINE }])
        : pageResult([])
    );
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    const loading = store.loadInitial();
    await vi.waitFor(() => expect(store.members.map((member) => member.id)).toEqual(['online']));
    expect(store.livePresence.get('online')).toBe(PresenceStatus.AWAY);
    expect(store.hasFirstPage).toBe(true);
    expect(store.hasLoadedAll).toBe(false);
    first.resolve(pageResult([user('offline')], true, 2000));
    await vi.waitFor(() => expect(store.totalCount).toBe(2000));
    expect(store.members.map((member) => member.id)).toContain('online');
    last.resolve(pageResult([user('online')], false, 2000));
    await loading;
    expect(store.members).toHaveLength(2);
    expect(store.hasLoadedAll).toBe(true);
  });

  it('keeps a newer offline event when the online preview arrives', async () => {
    const full = deferred<MemberDirectoryPage>();
    const preview = deferred<MemberDirectoryPage>();
    const api = new FakeMemberDirectoryAPI([full.promise]);
    api.listOnlineRoomMembers = vi.fn(async (_room, status) =>
      status === PresenceStatus.ONLINE ? preview.promise : pageResult([])
    );
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    const loading = store.loadInitial();
    store.setPresence('u', PresenceStatus.OFFLINE);
    preview.resolve(pageResult([user('u')]));
    await vi.waitFor(() => expect(store.members).toHaveLength(1));
    expect(store.livePresence.get('u')).toBe(PresenceStatus.OFFLINE);
    full.resolve(pageResult([user('u')]));
    await loading;
  });

  it('pages each presence group by consumed IDs and keeps mentions independent', async () => {
    const full = deferred<MemberDirectoryPage>();
    const api = new FakeMemberDirectoryAPI([full.promise, pageResult([user('mention')])]);
    api.listOnlineRoomMembers = vi.fn(async (_room, status, _limit, offset) => {
      if (status !== PresenceStatus.ONLINE) return pageResult([]);
      return offset === 0
        ? { members: [], consumedCount: 250, hasMore: true, totalCount: 251 }
        : pageResult([user('last-online')]);
    });
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    const loading = store.loadInitial();
    await vi.waitFor(() => expect(store.members).toHaveLength(1));
    expect(api.listOnlineRoomMembers).toHaveBeenCalledWith(
      'room',
      PresenceStatus.ONLINE,
      250,
      250,
      {}
    );
    expect((await store.searchMembers('mention')).map((member) => member.id)).toEqual(['mention']);
    full.resolve(pageResult([user('last-online'), user('mention')]));
    await loading;
  });

  it('ignores preview results after full completion or reset and tolerates preview failure', async () => {
    for (const reset of [false, true]) {
      const preview = deferred<MemberDirectoryPage>();
      const api = new FakeMemberDirectoryAPI([pageResult([user('canonical')])]);
      api.listOnlineRoomMembers = vi.fn(async (_room, status) => {
        if (status === PresenceStatus.ONLINE) return preview.promise;
        throw new Error('preview unavailable');
      });
      const store = new RoomMembersStore(api);
      store.setRoom('room');
      await store.loadInitial();
      if (reset) store.resetProjectionState();
      preview.resolve(pageResult([user('stale')]));
      await Promise.resolve();
      await Promise.resolve();
      expect(store.members.map((member) => member.id)).toEqual(reset ? [] : ['canonical']);
    }
  });
  it('restarts an in-flight offset scan at the membership event boundary', async () => {
    const oldPage = deferred<MemberDirectoryPage>();
    const api = new FakeMemberDirectoryAPI([
      oldPage.promise,
      pageResult([user('first'), user('joined')])
    ]);
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    const oldLoad = store.loadInitial();
    await store.applyMembership('joined', true, 'event-boundary');
    expect(api.listRoomMembers).toHaveBeenNthCalledWith(2, 'room', '', 250, 0, {
      minimumCursor: 'event-boundary'
    });
    oldPage.resolve(pageResult([user('first')]));
    await oldLoad;
    expect(store.members.map((member) => member.id)).toEqual(['first', 'joined']);
    expect(store.totalCount).toBe(2);
  });
  it('resolves mention names before the initial directory page finishes', async () => {
    const initial = deferred<MemberDirectoryPage>();
    const api = new FakeMemberDirectoryAPI([
      initial.promise,
      pageResult([user('far-away-id', 'zelda')])
    ]);
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    const loading = store.loadInitial();
    const matches = await store.searchMembers('zel');
    expect(matches.map((member) => member.login)).toEqual(['zelda']);
    expect(store.hasFirstPage).toBe(false);
    expect(api.listRoomMembers).toHaveBeenNthCalledWith(2, 'room', 'zel', 10, 0);
    initial.resolve(pageResult([user('first', 'alice')], false, 1));
    await loading;
  });
  it('advances by membership IDs when a profile disappears between list and hydration', async () => {
    const api = new FakeMemberDirectoryAPI([
      { members: [], consumedCount: 250, hasMore: true, totalCount: 251 },
      pageResult([user('last')], false, 251)
    ]);
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    await store.loadInitial();
    expect(api.listRoomMembers).toHaveBeenNthCalledWith(2, 'room', '', 250, 250);
    expect(store.members.map((member) => member.id)).toEqual(['last']);
  });

  it('applies joins, leaves, and profile updates without relisting a complete room', async () => {
    const api = new FakeMemberDirectoryAPI([pageResult([user('first')])]);
    api.batchGetUsers = vi.fn(async () => [user('second')]);
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    await store.loadInitial();
    await store.applyMembership('second', true);
    await store.applyMembership('second', true);
    store.updateUsers([user('first', 'renamed')]);
    expect(store.members.map((member) => member.login)).toEqual(['renamed', 'second']);
    expect(store.totalCount).toBe(2);
    await store.applyMembership('first', false);
    expect(store.members.map((member) => member.id)).toEqual(['second']);
    expect(store.totalCount).toBe(1);
    expect(api.listRoomMembers).toHaveBeenCalledTimes(1);
    expect(api.batchGetUsers).toHaveBeenCalledTimes(1);
  });

  it('retains a join ID while the shared profile loads without a blank row or second request', async () => {
    const api = new FakeMemberDirectoryAPI([pageResult([user('first')])]);
    const profiles = getUserStore('member-join-test', 'connection');
    profiles.set('first', new DirectoryMember({
      user: { id: 'first', login: 'first', displayName: 'First' }
    }));
    const connection = {
      serverId: 'member-join-test', queryScope: 'connection', getAPI: () => api
    } as unknown as ServerConnection;
    const store = new RoomMembersStore(connection);
    store.setRoom('room');
    await store.loadInitial();

    await store.applyMembership('second', true, 'join-cursor');
    expect(store.members.map((member) => member.id)).toEqual(['first']);
    expect(store.totalCount).toBe(2);
    expect(api.batchGetUsers).not.toHaveBeenCalled();

    profiles.set('second', new DirectoryMember({
      user: { id: 'second', login: 'second', displayName: 'Second' }
    }));
    expect(store.members.map((member) => member.login)).toEqual(['first', 'second']);

    await store.applyMembership('third', true, 'later-cursor');
    await store.applyMembership('third', false);
    profiles.set('third', new DirectoryMember({
      user: { id: 'third', login: 'third', displayName: 'Third' }
    }));
    expect(store.members.map((member) => member.id)).toEqual(['first', 'second']);

    disposeUserStore('member-join-test', 'connection');
  });

  it('keeps connected member names owned by the shared profile store', async () => {
    const api = new FakeMemberDirectoryAPI([pageResult([user('first', 'stale')])]);
    const profiles = getUserStore('member-profile-test', 'connection');
    profiles.set('first', new DirectoryMember({
      user: { id: 'first', login: 'current', displayName: 'Current' }
    }));
    const connection = {
      serverId: 'member-profile-test', queryScope: 'connection', getAPI: () => api
    } as unknown as ServerConnection;
    const store = new RoomMembersStore(connection);
    store.setRoom('room');
    await store.loadInitial();
    expect(store.members[0]?.login).toBe('current');

    store.updateUsers([user('first', 'stale')]);
    expect(store.members[0]?.login).toBe('current');

    profiles.set('first', new DirectoryMember({
      user: { id: 'first', login: 'updated', displayName: 'Updated' }
    }));
    expect(store.members[0]?.login).toBe('updated');
    disposeUserStore('member-profile-test', 'connection');
  });

  it('keeps a page member ID until its shared profile arrives', async () => {
    const api = new FakeMemberDirectoryAPI([
      {
        members: [],
        memberIds: ['late-profile'],
        consumedCount: 1,
        totalCount: 1,
        hasMore: false
      }
    ]);
    const profiles = getUserStore('late-member-profile-test', 'connection');
    const connection = {
      serverId: 'late-member-profile-test',
      queryScope: 'connection',
      getAPI: () => api
    } as unknown as ServerConnection;
    const store = new RoomMembersStore(connection);
    store.setRoom('room');
    await store.loadInitial();

    expect(store.members).toEqual([]);
    expect(store.totalCount).toBe(1);

    profiles.set(
      'late-profile',
      new DirectoryMember({
        user: { id: 'late-profile', login: 'alice', displayName: 'Alice Smith' }
      })
    );
    expect(store.members.map((member) => member.displayName)).toEqual(['Alice Smith']);
    disposeUserStore('late-member-profile-test', 'connection');
  });

  it('renders deleted members but omits identities with pending profiles', async () => {
    const api = new FakeMemberDirectoryAPI([{
      members: [],
      memberIds: ['deleted', 'pending'],
      consumedCount: 2,
      totalCount: 2,
      hasMore: false
    }]);
    const profiles = getUserStore('deleted-member-test', 'connection');
    profiles.delete('deleted');
    const connection = {
      serverId: 'deleted-member-test', queryScope: 'connection', getAPI: () => api
    } as unknown as ServerConnection;
    const store = new RoomMembersStore(connection);
    store.setRoom('room');
    await store.loadInitial();

    expect(store.members).toEqual([{
      id: 'deleted',
      login: '',
      displayName: '',
      deleted: true,
      avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE
    }]);
    expect(store.totalCount).toBe(2);

    profiles.set('pending', new DirectoryMember({
      user: { id: 'pending', login: 'alice', displayName: 'Alice' }
    }));
    expect(store.members.map((member) => member.displayName)).toEqual(['', 'Alice']);
    disposeUserStore('deleted-member-test', 'connection');
  });

  it('resolves a cached search ID after the shared profile arrives', async () => {
    const backgroundPage = deferred<MemberDirectoryPage>();
    const api = new FakeMemberDirectoryAPI([
      pageResult([user('first')], true, 2),
      backgroundPage.promise,
      { members: [], memberIds: ['late-profile'], consumedCount: 1, totalCount: 1, hasMore: false }
    ]);
    const profiles = getUserStore('late-search-profile-test', 'connection');
    profiles.set('first', new DirectoryMember({
      user: { id: 'first', login: 'first', displayName: 'First' }
    }));
    const connection = {
      serverId: 'late-search-profile-test',
      queryScope: 'connection',
      getAPI: () => api
    } as unknown as ServerConnection;
    const store = new RoomMembersStore(connection);
    store.setRoom('room');
    const loading = store.loadInitial();
    await vi.waitFor(() => expect(store.hasFirstPage).toBe(true));

    await store.setSearch('alice');
    expect(store.filteredMembers).toEqual([]);

    profiles.set('late-profile', new DirectoryMember({
      user: { id: 'late-profile', login: 'alice', displayName: 'Alice Smith' }
    }));
    expect(store.filteredMembers.map((member) => member.displayName)).toEqual(['Alice Smith']);

    backgroundPage.resolve(pageResult([user('late-profile', 'alice')], false, 2));
    await loading;
    disposeUserStore('late-search-profile-test', 'connection');
  });

  it('discards a pending join after that member leaves', async () => {
    const pending = deferred<ReturnType<typeof user>[]>();
    const api = new FakeMemberDirectoryAPI([pageResult([user('first')])]);
    api.batchGetUsers = vi.fn(() => pending.promise);
    const store = new RoomMembersStore(api);
    store.setRoom('room');
    await store.loadInitial();
    const join = store.applyMembership('second', true);
    await store.applyMembership('second', false);
    pending.resolve([user('second')]);
    await join;
    expect(store.members.map((member) => member.id)).toEqual(['first']);
  });
  it('requests room members in 250-member pages', () => {
    expect(ROOM_MEMBERS_PAGE_SIZE).toBe(250);
  });

  it('preserves explicit bot identity from directory members', async () => {
    const store = createStore([pageResult([user('bot-1', 'helper_bot', true)])]);
    store.setRoom('room-1');
    await store.loadInitial();

    expect(store.members[0]?.isBot).toBe(true);
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

  it('refresh keeps initial loading pending until the replacement first page arrives', async () => {
    const initial = deferred<MemberDirectoryPage>();
    const refresh = deferred<MemberDirectoryPage>();
    const store = createStore([initial.promise, refresh.promise]);

    store.setRoom('room-1');
    const initialLoad = store.loadInitial();
    expect(store.isInitialLoading).toBe(true);

    const refreshLoad = store.refresh();
    expect(store.isInitialLoading).toBe(true);

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

  it('distinguishes pending projection membership from a complete empty roster', () => {
    const store = new RoomMembersStore(null);

    store.awaitProjection('room-1');
    expect(store.isInitialLoading).toBe(true);
    expect(store.hasFirstPage).toBe(false);
    expect(store.hasLoadedAll).toBe(false);

    store.replaceProjection('room-1', []);
    expect(store.isInitialLoading).toBe(false);
    expect(store.hasFirstPage).toBe(true);
    expect(store.hasLoadedAll).toBe(true);
  });

  it.each([false, true])('handles a failed later page during reauthorization: %s', async (reauthorize) => {
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
      await store.refresh({ reauthorize });

      expect(store.members.map((member) => member.login)).toEqual(reauthorize ? [] : ['refresh-a']);
      expect(store.totalCount).toBe(reauthorize ? 0 : 3);
      expect(store.hasLoadedAll).toBe(false);
      expect(store.loadError).toBe('network failed');
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});
