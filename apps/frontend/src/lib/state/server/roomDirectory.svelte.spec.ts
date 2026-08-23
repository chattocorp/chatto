import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  RoomKind,
  type DirectoryRoomDetails,
  type RoomDirectoryAPI
} from '$lib/api-client/roomDirectory';
import { describe, expect, it, vi } from 'vitest';
import type { MemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import type { RoomsListItem } from './rooms.svelte';
import { RoomDirectoryStore, type RoomDirectoryNavigation } from './roomDirectory.svelte';

function room(id: string, member = false): RoomsListItem {
  return {
    id,
    name: id,
    description: `${id} description`,
    type: RoomKind.CHANNEL,
    isUniversal: false,
    viewerIsMember: member,
    viewerCanJoinRoom: true,
    viewerCanManageRoom: false,
    viewerDMArchived: false,
    viewerNotificationCount: 0,
    viewerImportantNotificationCount: 0,
    members: []
  };
}

function makeNavigation(rooms: RoomsListItem[] = [room('R1')]): RoomDirectoryNavigation {
  return {
    rooms,
    roomGroups: [],
    isInitialLoading: false,
    isRoomMember(roomId: string) {
      return this.rooms.some((candidate) => candidate.id === roomId && candidate.viewerIsMember);
    }
  };
}

function memberAPI(): Pick<MemberDirectoryAPI, 'listRoomMembers'> {
  return {
    listRoomMembers: vi.fn().mockResolvedValue({
      members: [
        {
          id: 'U1',
          login: 'ada',
          displayName: 'Ada',
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.ONLINE,
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 7,
      hasMore: true
    })
  };
}

function commands(
  overrides: Partial<Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'>> = {}
): Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'> {
  return {
    joinRoom: vi.fn().mockResolvedValue(null),
    leaveRoom: vi.fn().mockResolvedValue(true),
    joinGroup: vi.fn().mockResolvedValue([]),
    ...overrides
  };
}

function dmDetails(id: string, isDMArchived: boolean): DirectoryRoomDetails {
  return {
    id,
    name: '',
    description: null,
    kind: RoomKind.DM,
    archived: false,
    isUniversal: false,
    slowModeSeconds: 0,
    slowModeNextPostAt: null,
    isMember: true,
    hasUnread: false,
    isDMArchived,
    canJoinRoom: false,
    canManageRoom: false,
    canPostMessage: true,
    canPostInThread: false,
    canAttach: true,
    canReact: true,
    canEchoMessage: false,
    canManageOthersMessage: false,
    canBanRoomMembers: false
  };
}

function archiveCommands(
  overrides: Partial<Pick<RoomDirectoryAPI, 'archiveDM' | 'unarchiveDM'>> = {}
): Pick<RoomDirectoryAPI, 'archiveDM' | 'unarchiveDM'> {
  return {
    archiveDM: vi.fn(async (roomId) => dmDetails(roomId, true)),
    unarchiveDM: vi.fn(async (roomId) => dmDetails(roomId, false)),
    ...overrides
  };
}

function makeStore(
  navigation = makeNavigation(),
  api = commands(),
  directoryAPI = archiveCommands(),
  applyDMArchiveState = vi.fn()
): RoomDirectoryStore {
  return new RoomDirectoryStore(navigation, memberAPI(), api, directoryAPI, applyDMArchiveState);
}

describe('RoomDirectoryStore', () => {
  it('derives directory rows and membership from the navigation projection', () => {
    const navigation = makeNavigation([room('joined', true), room('open')]);
    const store = makeStore(navigation);

    expect(store.allRooms.map((candidate) => candidate.id)).toEqual(['joined', 'open']);
    expect(store.isJoined('joined')).toBe(true);
    expect(store.isJoined('open')).toBe(false);

    navigation.rooms = [room('open', true)];
    expect(store.allRooms.map((candidate) => candidate.id)).toEqual(['open']);
    expect(store.isJoined('open')).toBe(true);
  });

  it('keeps successful join and leave state optimistic until projection acknowledgement', async () => {
    const store = makeStore();

    expect((await store.joinRoom('R1')).ok).toBe(true);
    expect(store.isJoined('R1')).toBe(true);
    store.acknowledgeMembership('R1', true);
    expect(store.isJoined('R1')).toBe(false);

    const joinedNavigation = makeNavigation([room('R1', true)]);
    const joinedStore = makeStore(joinedNavigation);
    expect((await joinedStore.leaveRoom('R1')).ok).toBe(true);
    expect(joinedStore.isJoined('R1')).toBe(false);
    joinedStore.acknowledgeMembership('R1', false);
    expect(joinedStore.isJoined('R1')).toBe(true);
  });

  it('rolls command failures back without changing authoritative membership', async () => {
    const failure = new Error('nope');
    const store = makeStore(
      makeNavigation(),
      commands({ joinRoom: vi.fn().mockRejectedValue(failure) })
    );

    expect(await store.joinRoom('R1')).toEqual({ ok: false, error: failure });
    expect(store.isJoined('R1')).toBe(false);
    expect(store.joiningIds.size).toBe(0);
  });

  it('fences a late command response across projection reset', async () => {
    let resolveJoin!: () => void;
    const store = makeStore(
      makeNavigation(),
      commands({
        joinRoom: vi.fn(
          () =>
            new Promise<null>((resolve) => {
              resolveJoin = () => resolve(null);
            })
        )
      })
    );

    const joining = store.joinRoom('R1');
    store.resetOptimisticState();
    resolveJoin();
    await joining;

    expect(store.justJoinedIds.size).toBe(0);
    expect(store.joiningIds.size).toBe(0);
  });

  it('does not restore an optimistic overlay when projection acknowledgement wins the race', async () => {
    let resolveJoin!: () => void;
    const store = makeStore(
      makeNavigation(),
      commands({
        joinRoom: vi.fn(
          () =>
            new Promise<null>((resolve) => {
              resolveJoin = () => resolve(null);
            })
        )
      })
    );

    const joining = store.joinRoom('R1');
    store.acknowledgeMembership('R1', true);
    resolveJoin();
    await joining;

    expect(store.justJoinedIds.size).toBe(0);
  });

  it('keeps successful overlays until the projection confirms their value', async () => {
    const store = makeStore();

    await store.joinRoom('R1');
    store.acknowledgeMembership('R1', false);
    expect(store.isJoined('R1')).toBe(true);
    store.acknowledgeMembership('R1', true);
    expect(store.isJoined('R1')).toBe(false);

    const joinedNavigation = makeNavigation([room('R1', true)]);
    const joinedStore = makeStore(joinedNavigation);
    await joinedStore.leaveRoom('R1');
    joinedStore.acknowledgeMembership('R1', true);
    expect(joinedStore.isJoined('R1')).toBe(false);
    joinedStore.acknowledgeMembership('R1', false);
    expect(joinedStore.isJoined('R1')).toBe(true);
  });

  it('clears every membership overlay when the projected room is removed', async () => {
    const store = makeStore();
    await store.joinRoom('R1');
    expect(store.isJoined('R1')).toBe(true);

    store.removeMembershipProjection('R1');

    expect(store.isJoined('R1')).toBe(false);
    expect(store.justJoinedIds.size).toBe(0);
    expect(store.justLeftIds.size).toBe(0);
  });

  it('does not let an older command clear a newer pending marker', async () => {
    const resolvers: Array<() => void> = [];
    const store = makeStore(
      makeNavigation(),
      commands({
        joinRoom: vi.fn(
          () =>
            new Promise<null>((resolve) => {
              resolvers.push(() => resolve(null));
            })
        )
      })
    );

    const oldJoin = store.joinRoom('R1');
    store.resetOptimisticState();
    const newJoin = store.joinRoom('R1');
    resolvers[0]?.();
    await oldJoin;

    expect(store.joiningIds.has('R1')).toBe(true);

    resolvers[1]?.();
    await newJoin;
    expect(store.joiningIds.has('R1')).toBe(false);
  });

  it('loads join previews as an explicit best-effort query', async () => {
    const api = memberAPI();
    const store = new RoomDirectoryStore(
      makeNavigation(),
      api,
      commands(),
      archiveCommands(),
      vi.fn()
    );

    expect(await store.loadJoinPreview('R1')).toMatchObject({
      memberCount: 7,
      sampleMembers: [{ id: 'U1', displayName: 'Ada' }]
    });
    expect(api.listRoomMembers).toHaveBeenCalledWith('R1', '', 5, 0);
  });

  it('applies server-confirmed DM archive state and clears pending state', async () => {
    const api = archiveCommands();
    const apply = vi.fn();
    const store = makeStore(makeNavigation(), commands(), api, apply);

    await expect(store.setDMArchived('DM1', true)).resolves.toEqual({
      ok: true,
      isArchived: true
    });
    expect(api.archiveDM).toHaveBeenCalledWith('DM1');
    expect(apply).toHaveBeenCalledWith('DM1', true);
    expect(store.dmArchivePendingIds.size).toBe(0);

    await expect(store.setDMArchived('DM1', false)).resolves.toEqual({
      ok: true,
      isArchived: false
    });
    expect(api.unarchiveDM).toHaveBeenCalledWith('DM1');
    expect(apply).toHaveBeenLastCalledWith('DM1', false);
  });

  it('reports DM archive failures without changing projected state', async () => {
    const failure = new Error('archive failed');
    const apply = vi.fn();
    const store = makeStore(
      makeNavigation(),
      commands(),
      archiveCommands({ archiveDM: vi.fn().mockRejectedValue(failure) }),
      apply
    );

    await expect(store.setDMArchived('DM1', true)).resolves.toEqual({
      ok: false,
      error: failure
    });
    expect(apply).not.toHaveBeenCalled();
    expect(store.dmArchivePendingIds.size).toBe(0);
  });
});
