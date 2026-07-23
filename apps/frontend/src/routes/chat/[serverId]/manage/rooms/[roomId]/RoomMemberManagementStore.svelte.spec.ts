import { describe, expect, it, vi } from 'vitest';
import type {
  DirectoryMember,
  MemberDirectoryAPI,
  MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import { PresenceStatus } from '$lib/api-client/renderTypes';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import {
  ROOM_MEMBER_MANAGEMENT_PAGE_SIZE,
  RoomMemberManagementStore,
  type RoomMemberManagementAPIs
} from './RoomMemberManagementStore.svelte';

function member(id: string): DirectoryMember {
  return {
    id,
    login: id,
    displayName: id.toUpperCase(),
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline,
    customStatus: null,
    roles: ['everyone'],
    createdAt: null
  };
}

function page(
  members: DirectoryMember[],
  totalCount = members.length,
  hasMore = false
): MemberDirectoryPage {
  return { members, totalCount, hasMore };
}

function APIs(
  overrides: {
    listRoomMembers?: MemberDirectoryAPI['listRoomMembers'];
    listUsers?: MemberDirectoryAPI['listUsers'];
    batchGetRoomMembers?: MemberDirectoryAPI['batchGetRoomMembers'];
    addMember?: RoomCommandAPI['addMember'];
    removeMember?: RoomCommandAPI['removeMember'];
  } = {}
): RoomMemberManagementAPIs {
  return {
    directory: {
      listRoomMembers: overrides.listRoomMembers ?? vi.fn().mockResolvedValue(page([])),
      listUsers: overrides.listUsers ?? vi.fn().mockResolvedValue(page([])),
      batchGetRoomMembers: overrides.batchGetRoomMembers ?? vi.fn().mockResolvedValue([]),
      getUser: vi.fn(),
      getUserByLogin: vi.fn(),
      batchGetUsers: vi.fn(),
      getRoomMember: vi.fn()
    } as unknown as MemberDirectoryAPI,
    commands: {
      addMember: overrides.addMember ?? vi.fn().mockResolvedValue(null),
      removeMember: overrides.removeMember ?? vi.fn().mockResolvedValue(true)
    } as unknown as RoomCommandAPI
  };
}

describe('RoomMemberManagementStore', () => {
  it('loads members in offset pages without replacing previously loaded rows', async () => {
    const first = Array.from({ length: ROOM_MEMBER_MANAGEMENT_PAGE_SIZE }, (_, index) =>
      member(`user-${index}`)
    );
    const listRoomMembers = vi
      .fn()
      .mockResolvedValueOnce(page(first, 21, true))
      .mockResolvedValueOnce(page([member('user-20')], 21, false));
    const store = new RoomMemberManagementStore(() => APIs({ listRoomMembers }));

    store.setRoom('server-1', 'room-1');
    await store.loadFirstPage();
    await store.loadMore();

    expect(listRoomMembers).toHaveBeenNthCalledWith(
      1,
      'room-1',
      '',
      ROOM_MEMBER_MANAGEMENT_PAGE_SIZE,
      0
    );
    expect(listRoomMembers).toHaveBeenNthCalledWith(
      2,
      'room-1',
      '',
      ROOM_MEMBER_MANAGEMENT_PAGE_SIZE,
      ROOM_MEMBER_MANAGEMENT_PAGE_SIZE
    );
    expect(store.members).toHaveLength(21);
    expect(store.hasMore).toBe(false);
  });

  it('excludes existing room members from server-directory search results', async () => {
    const alice = member('alice');
    const bob = member('bob');
    const listUsers = vi.fn().mockResolvedValue(page([alice, bob]));
    const batchGetRoomMembers = vi.fn().mockResolvedValue([alice]);
    const store = new RoomMemberManagementStore(() => APIs({ listUsers, batchGetRoomMembers }));

    store.setRoom('server-1', 'room-1');
    await store.searchDirectory('a');

    expect(batchGetRoomMembers).toHaveBeenCalledWith('room-1', ['alice', 'bob']);
    expect(store.directoryResults).toEqual([bob]);
  });

  it('continues directory paging when the first page contains only existing members', async () => {
    const existing = Array.from({ length: 20 }, (_, index) => member(`existing-${index}`));
    const bob = member('bob');
    const listUsers = vi
      .fn()
      .mockResolvedValueOnce(page(existing, 21, true))
      .mockResolvedValueOnce(page([bob], 21, false));
    const batchGetRoomMembers = vi.fn().mockResolvedValueOnce(existing).mockResolvedValueOnce([]);
    const store = new RoomMemberManagementStore(() => APIs({ listUsers, batchGetRoomMembers }));

    store.setRoom('server-1', 'room-1');
    await store.searchDirectory('b');

    expect(listUsers).toHaveBeenNthCalledWith(1, 'b', 20, 0);
    expect(listUsers).toHaveBeenNthCalledWith(2, 'b', 20, 20);
    expect(store.directoryResults).toEqual([bob]);
  });

  it('does not retain or restore members when the server changes with the same room ID', async () => {
    let resolveFirst!: (value: MemberDirectoryPage) => void;
    const firstPage = new Promise<MemberDirectoryPage>((resolve) => {
      resolveFirst = resolve;
    });
    const listRoomMembers = vi
      .fn()
      .mockReturnValueOnce(firstPage)
      .mockResolvedValueOnce(page([member('bob')]));
    const store = new RoomMemberManagementStore(() => APIs({ listRoomMembers }));

    store.setRoom('server-1', 'shared-room');
    const staleLoad = store.loadFirstPage();
    store.setRoom('server-2', 'shared-room');
    await store.loadFirstPage();
    resolveFirst(page([member('alice')]));
    await staleLoad;

    expect(store.members).toEqual([member('bob')]);
  });

  it('purges room data and fences an older refresh at the access-loss boundary', async () => {
    let resolveRefresh!: (value: MemberDirectoryPage) => void;
    const refreshPage = new Promise<MemberDirectoryPage>((resolve) => {
      resolveRefresh = resolve;
    });
    const listRoomMembers = vi
      .fn()
      .mockResolvedValueOnce(page([member('alice')]))
      .mockReturnValueOnce(refreshPage);
    const store = new RoomMemberManagementStore(() => APIs({ listRoomMembers }));

    store.setRoom('server-1', 'room-1');
    await store.loadFirstPage();
    const staleRefresh = store.refresh();

    expect(store.clearRoom('server-1', 'room-1')).toBe(true);
    expect(store.members).toEqual([]);
    resolveRefresh(page([member('alice')]));
    await staleRefresh;

    expect(store.members).toEqual([]);
  });

  it('re-reads canonical membership after add and remove', async () => {
    const alice = member('alice');
    const bob = member('bob');
    let current = [alice];
    const listRoomMembers = vi.fn().mockImplementation(() => Promise.resolve(page(current)));
    const addMember = vi.fn().mockImplementation(async () => {
      current = [alice, bob];
      return bob;
    });
    const removeMember = vi.fn().mockImplementation(async () => {
      current = [bob];
      return true;
    });
    const api = APIs({ listRoomMembers, addMember, removeMember });
    const store = new RoomMemberManagementStore(() => api);

    store.setRoom('server-1', 'room-1');
    await store.loadFirstPage();
    expect(await store.addMember(bob)).toBe(true);
    expect(store.members).toEqual([alice, bob]);
    expect(await store.removeMember(alice)).toBe(true);

    expect(store.members).toEqual([bob]);
  });

  it('keeps command success authoritative when the canonical reread fails', async () => {
    const alice = member('alice');
    const bob = member('bob');
    const listRoomMembers = vi
      .fn()
      .mockResolvedValueOnce(page([alice]))
      .mockRejectedValueOnce(new Error('projection temporarily unavailable'));
    const addMember = vi.fn().mockResolvedValue(bob);
    const store = new RoomMemberManagementStore(() => APIs({ listRoomMembers, addMember }));

    store.setRoom('server-1', 'room-1');
    await store.loadFirstPage();

    expect(await store.addMember(bob)).toBe(true);
    expect(store.loadError).toBe('projection temporarily unavailable');
    expect(store.members).toEqual([alice]);
  });
});
