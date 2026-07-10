import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import {
  NotificationLevel,
  PresenceStatus,
  RoomType,
  type UserAvatarUserView
} from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { ROOM_MEMBERS_PAGE_SIZE } from '$lib/state/room/members.svelte';
import {
  RoomDirectoryScope,
  RoomKind,
  type DirectoryRoomGroup,
  type DirectoryRoomGroupItem,
  type DirectoryRoomSummary,
  type RoomDirectoryAPI
} from '$lib/api-client/roomDirectory';
import type { DirectoryMember, MemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import type { NotificationAPI } from '$lib/api-client/notifications';
import type { ViewerState } from '$lib/api-client/viewer';
import { NotificationLevelStore } from './notificationLevel.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { isRoomStateRefreshEvent, RoomsStore, type ViewerStateLoader } from './rooms.svelte';

function makeRoom(id: string, overrides: Partial<DirectoryRoomSummary> = {}): DirectoryRoomSummary {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    kind: overrides.kind ?? RoomKind.CHANNEL,
    archived: overrides.archived ?? false,
    isUniversal: overrides.isUniversal ?? false,
    isMember: overrides.isMember ?? true,
    hasUnread: overrides.hasUnread ?? false,
    canJoinRoom: overrides.canJoinRoom ?? true
  };
}

function makeGroupRoomItem(
  id: string,
  overrides: Partial<DirectoryRoomSummary> = {}
): DirectoryRoomGroupItem {
  return {
    id: `room:${id}`,
    type: 'room',
    roomId: id,
    room: makeRoom(id, overrides)
  };
}

function makeMember(id: string, overrides: Partial<DirectoryMember> = {}): DirectoryMember {
  return {
    id,
    login: overrides.login ?? id.toLowerCase(),
    displayName: overrides.displayName ?? id,
    deleted: overrides.deleted ?? false,
    avatarUrl: overrides.avatarUrl ?? null,
    presenceStatus: overrides.presenceStatus ?? PresenceStatus.Online,
    customStatus: overrides.customStatus ?? null,
    roles: overrides.roles ?? [],
    createdAt: overrides.createdAt ?? null
  };
}

function makeViewer(overrides: Partial<ViewerState> = {}): ViewerState {
  return {
    user: {
      id: 'U1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      customStatus: null,
      presenceStatus: PresenceStatus.Online,
      hasVerifiedEmail: true,
      hasPassword: true,
      viewerCanDeleteAccount: true,
      lastLoginChange: null,
      settings: null
    },
    canViewAdmin: false,
    canStartDMs: true,
    canAdminViewUsers: false,
    canAdminManageAccounts: false,
    canAssignRoles: false,
    canAdminViewRoles: false,
    canAdminManageRoles: false,
    canAdminViewSystem: false,
    canAdminViewAudit: false,
    canManageUserPermissions: false,
    serverNotificationPreference: {
      level: NotificationLevel.Default,
      effectiveLevel: NotificationLevel.Normal
    },
    roomNotificationPreferences: [],
    viewerPermissions: {},
    viewerHasUnreadRooms: false,
    ...overrides
  };
}

function makeNotificationAPI(counts: Record<string, number> = {}): NotificationAPI {
  return {
    listNotifications: vi.fn(),
    listRoomNotifications: vi.fn(),
    hasNotifications: vi.fn(),
    listRoomNotificationCounts: vi.fn().mockResolvedValue(counts),
    listNotificationCounts: vi.fn().mockResolvedValue(counts),
    dismissNotification: vi.fn(),
    dismissAllNotifications: vi.fn()
  } as unknown as NotificationAPI;
}

function makeRoomDirectoryAPI(
  rooms: DirectoryRoomSummary[] = [],
  groups: DirectoryRoomGroup[] = []
): RoomDirectoryAPI {
  return {
    listRooms: vi.fn().mockResolvedValue(rooms),
    listRoomGroups: vi.fn().mockResolvedValue(groups)
  } as unknown as RoomDirectoryAPI;
}

function makeMemberDirectoryAPI(
  membersByRoomId: Record<string, DirectoryMember[]> = {}
): MemberDirectoryAPI {
  return {
    listUsers: vi.fn(),
    listRoomMembers: vi.fn(async (roomId: string) => ({
      members: membersByRoomId[roomId] ?? [],
      totalCount: membersByRoomId[roomId]?.length ?? 0,
      hasMore: false
    }))
  } as unknown as MemberDirectoryAPI;
}

function makeStore({
  roomDirectoryAPI = makeRoomDirectoryAPI(),
  memberDirectoryAPI = makeMemberDirectoryAPI(),
  viewerStateLoader = vi.fn().mockResolvedValue(makeViewer()),
  notificationAPI = makeNotificationAPI(),
  notificationLevels = new NotificationLevelStore(),
  roomUnread = new RoomUnreadStore()
}: {
  roomDirectoryAPI?: RoomDirectoryAPI;
  memberDirectoryAPI?: MemberDirectoryAPI;
  viewerStateLoader?: ViewerStateLoader;
  notificationAPI?: NotificationAPI;
  notificationLevels?: NotificationLevelStore;
  roomUnread?: RoomUnreadStore;
} = {}) {
  return new RoomsStore(
    roomDirectoryAPI,
    memberDirectoryAPI,
    viewerStateLoader,
    notificationLevels,
    roomUnread,
    notificationAPI
  );
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('RoomsStore - refresh', () => {
  it('loads listable non-member channels from room directory and DMs with members', async () => {
    const roomDirectoryAPI = makeRoomDirectoryAPI(
      [
        makeRoom('public', { isMember: false, canJoinRoom: true }),
        makeRoom('dm-1', { kind: RoomKind.DM, name: '' })
      ],
      [
        {
          id: 'g1',
          name: 'Lobby',
          canCreateRoom: false,
          roomIds: ['public'],
          items: [makeGroupRoomItem('public')]
        }
      ]
    );
    const memberDirectoryAPI = makeMemberDirectoryAPI({
      'dm-1': [makeMember('U1', { login: 'alice', displayName: 'Alice' })]
    });
    const store = makeStore({ roomDirectoryAPI, memberDirectoryAPI });

    await store.refresh();

    expect(roomDirectoryAPI.listRooms).toHaveBeenCalledWith(RoomDirectoryScope.ALL);
    expect(memberDirectoryAPI.listRoomMembers).toHaveBeenCalledWith(
      'dm-1',
      '',
      ROOM_MEMBERS_PAGE_SIZE,
      0
    );
    expect(store.rooms).toMatchObject([
      {
        id: 'public',
        type: RoomType.Channel,
        isUniversal: false,
        viewerIsMember: false,
        viewerCanJoinRoom: true,
        members: []
      },
      {
        id: 'dm-1',
        type: RoomType.Dm,
        isUniversal: false,
        viewerIsMember: true,
        members: [{ id: 'U1', displayName: 'Alice' }]
      }
    ]);
    expect(store.roomGroups).toMatchObject([
      {
        id: 'g1',
        items: [{ id: 'room:public', type: 'room', roomId: 'public' }]
      }
    ]);
  });

  it('maps universal channel rooms from the room directory', async () => {
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general', { isUniversal: true })])
    });

    await store.refresh();

    expect(store.rooms).toMatchObject([{ id: 'general', isUniversal: true }]);
  });

  it('loads viewer identity and notification preferences from Connect viewer state', async () => {
    const notificationLevels = new NotificationLevelStore();
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general')]),
      notificationLevels,
      viewerStateLoader: vi.fn().mockResolvedValue(
        makeViewer({
          user: {
            ...makeViewer().user,
            id: 'U2'
          },
          serverNotificationPreference: {
            level: NotificationLevel.Muted,
            effectiveLevel: NotificationLevel.Muted
          },
          roomNotificationPreferences: [
            {
              roomId: 'general',
              level: NotificationLevel.AllMessages,
              effectiveLevel: NotificationLevel.AllMessages
            }
          ]
        })
      )
    });

    await store.refresh();

    expect(store.currentUserId).toBe('U2');
    expect(notificationLevels.getServerPreference()).toEqual({
      level: NotificationLevel.Muted,
      effectiveLevel: NotificationLevel.Muted
    });
    expect(notificationLevels.getRoomPreference('general')).toEqual({
      level: NotificationLevel.AllMessages,
      effectiveLevel: NotificationLevel.AllMessages
    });
  });

  it('discards out-of-order responses', async () => {
    let resolveFirstRooms!: (value: DirectoryRoomSummary[]) => void;
    let resolveSecondRooms!: (value: DirectoryRoomSummary[]) => void;
    const listRooms = vi.fn().mockImplementation(() => {
      if (listRooms.mock.calls.length === 1) {
        return new Promise<DirectoryRoomSummary[]>((resolve) => (resolveFirstRooms = resolve));
      }
      return new Promise<DirectoryRoomSummary[]>((resolve) => (resolveSecondRooms = resolve));
    });
    const listRoomGroups = vi.fn().mockImplementation(() => {
      if (listRoomGroups.mock.calls.length === 1) {
        return Promise.resolve([
          {
            id: 'g1',
            name: 'Lobby',
            roomIds: ['older'],
            items: [makeGroupRoomItem('older')]
          }
        ]);
      }
      return Promise.resolve([
        {
          id: 'g1',
          name: 'Lobby',
          roomIds: ['newer'],
          items: [makeGroupRoomItem('newer')]
        }
      ]);
    });
    const roomDirectoryAPI = {
      listRooms,
      listRoomGroups
    } as unknown as RoomDirectoryAPI;
    const store = makeStore({
      roomDirectoryAPI,
      notificationAPI: makeNotificationAPI({ newer: 4 })
    });

    void store.refresh();
    void store.refresh();

    resolveSecondRooms([makeRoom('newer')]);
    await settle();

    expect(store.rooms.map((room) => room.id)).toEqual(['newer']);
    expect(store.roomGroups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        roomIds: ['newer'],
        items: [{ id: 'room:newer', type: 'room', roomId: 'newer' }]
      }
    ]);
    await vi.waitFor(() => {
      expect(store.rooms.find((room) => room.id === 'newer')?.viewerNotificationCount).toBe(4);
    });

    resolveFirstRooms([makeRoom('older')]);
    await settle();

    expect(store.rooms.map((room) => room.id)).toEqual(['newer']);
    expect(store.roomGroups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        roomIds: ['newer'],
        items: [{ id: 'room:newer', type: 'room', roomId: 'newer' }]
      }
    ]);
  });

  it('patches notification counts from Connect', async () => {
    let resolveCounts!: (value: Record<string, number>) => void;
    const notificationAPI = makeNotificationAPI();
    vi.mocked(notificationAPI.listNotificationCounts).mockImplementation(
      () => new Promise((resolve) => (resolveCounts = resolve))
    );
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general')]),
      notificationAPI
    });

    await store.refresh();

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 0 }]);
    resolveCounts({ general: 7 });

    await vi.waitFor(() => {
      expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 7 }]);
    });
  });

  it('refreshes notification counts for an already-loaded room list', async () => {
    let countQueries = 0;
    const notificationAPI = makeNotificationAPI();
    vi.mocked(notificationAPI.listNotificationCounts).mockImplementation(async () => {
      countQueries++;
      return { general: countQueries === 1 ? 1 : 0 };
    });
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general')]),
      notificationAPI
    });

    await store.refresh();
    await vi.waitFor(() => {
      expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 1 }]);
    });

    await store.refreshNotificationCounts();

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 0 }]);
  });

  it('preserves unchanged room rows and array identity during refresh', async () => {
    const roomDirectoryAPI = makeRoomDirectoryAPI([makeRoom('general'), makeRoom('random')]);
    const store = makeStore({
      roomDirectoryAPI,
      notificationAPI: makeNotificationAPI({ general: 1, random: 0 })
    });

    await store.refresh();
    await vi.waitFor(() => {
      expect(store.rooms[0]?.viewerNotificationCount).toBe(1);
    });

    const rooms = store.rooms;
    const general = store.rooms[0];
    const random = store.rooms[1];
    vi.mocked(roomDirectoryAPI.listRooms).mockResolvedValue([
      makeRoom('general'),
      makeRoom('random')
    ]);

    await store.refresh();
    await settle();

    expect(store.rooms).toBe(rooms);
    expect(store.rooms[0]).toBe(general);
    expect(store.rooms[1]).toBe(random);
  });

  it('refreshes unread state without replacing room rows', async () => {
    const roomDirectoryAPI = makeRoomDirectoryAPI([makeRoom('general'), makeRoom('random')]);
    const roomUnread = new RoomUnreadStore();
    const store = makeStore({
      roomDirectoryAPI,
      notificationAPI: makeNotificationAPI(),
      roomUnread
    });

    await store.refresh();
    await settle();

    const general = store.rooms[0];
    const random = store.rooms[1];
    vi.mocked(roomDirectoryAPI.listRooms).mockResolvedValue([
      makeRoom('general'),
      makeRoom('random', { hasUnread: true })
    ]);

    await store.refresh();
    await settle();

    expect(store.rooms[0]).toBe(general);
    expect(store.rooms[1]).toBe(random);
    expect(roomUnread.roomIsUnread('random')).toBe(true);
  });

  it('does not cancel an optimistic read when an older refresh completes', async () => {
    let resolveRooms!: (value: DirectoryRoomSummary[]) => void;
    const roomDirectoryAPI = {
      listRooms: vi.fn(
        () => new Promise<DirectoryRoomSummary[]>((resolve) => (resolveRooms = resolve))
      ),
      listRoomGroups: vi.fn().mockResolvedValue([])
    } as unknown as RoomDirectoryAPI;
    const roomUnread = new RoomUnreadStore();
    roomUnread.setRoomUnread('general', true);
    const store = makeStore({ roomDirectoryAPI, roomUnread });

    const refresh = store.refresh();
    const read = roomUnread.beginOptimisticRead('general');
    resolveRooms([makeRoom('general', { hasUnread: true })]);
    await refresh;

    expect(roomUnread.roomIsUnread('general')).toBe(false);

    read.commit();

    expect(roomUnread.roomIsUnread('general')).toBe(false);
  });

  it('only replaces rooms whose notification counts changed', async () => {
    const notificationAPI = makeNotificationAPI({ general: 1, random: 0 });
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general'), makeRoom('random')]),
      notificationAPI
    });

    await store.refresh();
    await vi.waitFor(() => {
      expect(store.rooms[0]?.viewerNotificationCount).toBe(1);
    });

    const rooms = store.rooms;
    const general = store.rooms[0];
    const random = store.rooms[1];

    await store.refreshNotificationCounts();

    expect(store.rooms).toBe(rooms);
    expect(store.rooms[0]).toBe(general);
    expect(store.rooms[1]).toBe(random);

    vi.mocked(notificationAPI.listNotificationCounts).mockResolvedValue({ general: 1, random: 3 });

    await store.refreshNotificationCounts();

    expect(store.rooms).not.toBe(rooms);
    expect(store.rooms[0]).toBe(general);
    expect(store.rooms[1]).not.toBe(random);
    expect(store.rooms[1]).toMatchObject({ id: 'random', viewerNotificationCount: 3 });
  });

  it('discards out-of-order notification count refresh responses', async () => {
    let countQueries = 0;
    let resolveOlder!: (value: Record<string, number>) => void;
    let resolveNewer!: (value: Record<string, number>) => void;
    const notificationAPI = makeNotificationAPI();
    vi.mocked(notificationAPI.listNotificationCounts).mockImplementation(() => {
      countQueries++;
      if (countQueries === 1) return Promise.resolve({ general: 0 });
      if (countQueries === 2) return new Promise((resolve) => (resolveOlder = resolve));
      return new Promise((resolve) => (resolveNewer = resolve));
    });
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general')]),
      notificationAPI
    });

    await store.refresh();
    await vi.waitFor(() => {
      expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 0 }]);
    });

    const olderRefresh = store.refreshNotificationCounts();
    const newerRefresh = store.refreshNotificationCounts();

    resolveNewer({ general: 2 });
    await newerRefresh;

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 2 }]);

    resolveOlder({ general: 1 });
    await olderRefresh;

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 2 }]);
  });

  it('keeps rooms visible when notification count loading fails', async () => {
    const notificationAPI = makeNotificationAPI();
    vi.mocked(notificationAPI.listNotificationCounts).mockRejectedValue(
      new Error('server too old')
    );
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('general')]),
      notificationAPI
    });

    await store.refresh();
    await settle();

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 0 }]);
  });

  it('maps mixed sidebar group items from the room directory', async () => {
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI(
        [makeRoom('general')],
        [
          {
            id: 'g1',
            name: 'Lobby',
            canCreateRoom: false,
            roomIds: ['general'],
            items: [
              {
                id: 'link:docs',
                type: 'link',
                link: { id: 'docs', label: 'Docs', url: 'https://example.com/docs' }
              },
              makeGroupRoomItem('general')
            ]
          }
        ]
      )
    });

    await store.refresh();

    expect(store.roomGroups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        roomIds: ['general'],
        items: [
          {
            id: 'link:docs',
            type: 'link',
            link: { id: 'docs', label: 'Docs', url: 'https://example.com/docs' }
          },
          { id: 'room:general', type: 'room', roomId: 'general' }
        ]
      }
    ]);
  });

  it('preserves the avatar shape expected by sidebar DM rows', async () => {
    const member: DirectoryMember = makeMember('U2', {
      login: 'bob',
      displayName: 'Bob',
      customStatus: { emoji: ':wave:', text: 'Hi', expiresAt: null }
    });
    const store = makeStore({
      roomDirectoryAPI: makeRoomDirectoryAPI([makeRoom('dm-1', { kind: RoomKind.DM })]),
      memberDirectoryAPI: makeMemberDirectoryAPI({ 'dm-1': [member] })
    });

    await store.refresh();

    expect(store.rooms[0]?.members).toEqual<UserAvatarUserView[]>([
      {
        id: 'U2',
        login: 'bob',
        displayName: 'Bob',
        deleted: false,
        avatarUrl: null,
        presenceStatus: PresenceStatus.Online,
        customStatus: {
          emoji: ':wave:',
          text: 'Hi',
          expiresAt: null
        }
      }
    ]);
  });
});

describe('RoomsStore - ingestServerEvent', () => {
  function makeEvent(kind: RoomEventKind, extra: Record<string, unknown> = {}) {
    return { event: { kind, ...extra } } as never;
  }

  it('uses one shared predicate for room state refresh events', () => {
    expect(isRoomStateRefreshEvent({ kind: RoomEventKind.RoomCreated } as never)).toBe(true);
    expect(isRoomStateRefreshEvent({ kind: RoomEventKind.RoomGroupsUpdated } as never)).toBe(true);
    expect(isRoomStateRefreshEvent({ kind: RoomEventKind.RoomUniversalChanged } as never)).toBe(
      true
    );
    expect(isRoomStateRefreshEvent({ kind: RoomEventKind.ReactionAdded } as never)).toBe(false);
  });

  it('refreshes on RoomCreatedEvent', () => {
    const store = makeStore();
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent(RoomEventKind.RoomCreated));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes on RoomGroupsUpdatedEvent', () => {
    const store = makeStore();
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent(RoomEventKind.RoomGroupsUpdated));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes on UserJoinedRoomEvent', () => {
    const store = makeStore();
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent(RoomEventKind.UserJoinedRoom));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('does not refresh on irrelevant event types', () => {
    const store = makeStore();
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent(RoomEventKind.ReactionAdded));
    store.ingestServerEvent(makeEvent(RoomEventKind.Heartbeat));

    expect(store.refresh).not.toHaveBeenCalled();
  });
});
