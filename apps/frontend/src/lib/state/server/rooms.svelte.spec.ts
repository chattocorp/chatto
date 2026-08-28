import { describe, expect, it } from 'vitest';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { PermissionGrant } from '@chatto/api-types/api/v1/permissions_pb';
import {
  RoomGroup,
  RoomGroupItem,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { Room, RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { GetViewerResponse, ViewerUser } from '@chatto/api-types/api/v1/viewer_pb';
import { RealtimeProjectionRoom } from '@chatto/api-types/realtime/v1/realtime_pb';
import { ServerProjectionStore } from './projection.svelte';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';
import { isNavigationVisibleRoom, NavigationStore } from './rooms.svelte';

function navigationFor(
  projection: ServerProjectionStore,
  sync = new RealtimeProjectionSyncState(),
  notificationCounts = { roomUnreadCounts: {}, roomImportantUnreadCounts: {} }
): { navigation: NavigationStore; sync: RealtimeProjectionSyncState } {
  sync.markCaughtUp(undefined);
  return { navigation: new NavigationStore(projection, sync, notificationCounts), sync };
}

function projectedRoom(
  id: string,
  {
    kind = RoomKind.CHANNEL,
    member = true,
    memberUserIds = [],
    hasMessageHistory,
    canReadMessages = true
  }: {
    kind?: RoomKind;
    member?: boolean;
    memberUserIds?: string[];
    hasMessageHistory?: boolean;
    canReadMessages?: boolean;
  } = {}
): RealtimeProjectionRoom {
  return new RealtimeProjectionRoom({
    room: new RoomWithViewerState({
      room: new Room({ id, name: id, kind }),
      viewerState: new RoomViewerState({
        isMember: member,
        permissions: [
          new PermissionGrant({ permission: 'room.join', granted: true }),
          new PermissionGrant({ permission: 'room.manage', granted: id === 'managed' }),
          new PermissionGrant({ permission: 'message.read', granted: canReadMessages })
        ]
      })
    }),
    memberUserIds,
    hasMessageHistory
  });
}

describe('NavigationStore', () => {
  it('selects rooms, members, permissions, counts, and viewer identity from the projection', () => {
    const projection = new ServerProjectionStore();
    projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) })
    });
    projection.users.set(
      'U2',
      new DirectoryMember({
        user: new User({ id: 'U2', login: 'ada', displayName: 'Ada' })
      })
    );
    projection.rooms.set(
      'dm',
      projectedRoom('dm', {
        kind: RoomKind.DM,
        memberUserIds: ['U2'],
        hasMessageHistory: true
      })
    );
    projection.rooms.set('managed', projectedRoom('managed'));

    const { navigation } = navigationFor(projection, undefined, {
      roomUnreadCounts: { dm: 3 },
      roomImportantUnreadCounts: { dm: 2 }
    });

    expect(navigation.currentUserId).toBe('U1');
    expect(navigation.isInitialLoading).toBe(false);
    expect(navigation.rooms).toMatchObject([
      {
        id: 'dm',
        type: RoomKind.DM,
        viewerNotificationCount: 3,
        viewerImportantNotificationCount: 2,
        hasMessageHistory: true,
        members: [{ id: 'U2', displayName: 'Ada' }]
      },
      {
        id: 'managed',
        viewerCanJoinRoom: true,
        viewerCanManageRoom: true
      }
    ]);
  });

  it('preserves projection ordering and derives room groups without retaining copies', () => {
    const projection = new ServerProjectionStore();
    projection.rooms.set('older', projectedRoom('older'));
    projection.rooms.set('newer', projectedRoom('newer'));
    projection.roomGroups = [
      new RoomGroup({
        id: 'G1',
        name: 'Projects',
        items: [
          new RoomGroupItem({
            item: { case: 'room', value: projectedRoom('newer').room! }
          })
        ]
      })
    ];
    const { navigation } = navigationFor(projection);

    expect(navigation.rooms.map((room) => room.id)).toEqual(['older', 'newer']);
    expect(navigation.roomGroups).toMatchObject([
      { id: 'G1', name: 'Projects', roomIds: ['newer'] }
    ]);

    projection.rooms.delete('older');
    expect(navigation.rooms.map((room) => room.id)).toEqual(['newer']);
  });

  it('uses DM history instead of message.read to control DM navigation', () => {
    const projection = new ServerProjectionStore();
    projection.rooms.set(
      'unreadable-dm',
      projectedRoom('unreadable-dm', {
        kind: RoomKind.DM,
        hasMessageHistory: true,
        canReadMessages: false
      })
    );
    const { navigation } = navigationFor(projection);

    expect(navigation.rooms.map((room) => room.id)).toEqual(['unreadable-dm']);
    expect(isNavigationVisibleRoom(navigation.rooms[0])).toBe(true);
  });

  it('treats a missing Important count as zero after the last Important occurrence is read', () => {
    const projection = new ServerProjectionStore();
    projection.rooms.set('ambient', projectedRoom('ambient'));
    const { navigation } = navigationFor(projection, undefined, {
      roomUnreadCounts: { ambient: 1 },
      roomImportantUnreadCounts: {}
    });

    expect(navigation.rooms[0]).toMatchObject({
      viewerNotificationCount: 1,
      viewerImportantNotificationCount: 0
    });
  });

  it('becomes empty immediately when the canonical projection resets', () => {
    const projection = new ServerProjectionStore();
    projection.viewer = new GetViewerResponse();
    projection.rooms.set('R1', projectedRoom('R1'));
    const { navigation, sync } = navigationFor(projection);

    projection.reset();
    sync.acceptProjectionEvent(undefined, true);

    expect(navigation.rooms).toEqual([]);
    expect(navigation.roomGroups).toEqual([]);
    expect(navigation.isInitialLoading).toBe(true);
  });

  it('hides a compacted projection prefix until caught up while retaining stale state', () => {
    const projection = new ServerProjectionStore();
    const sync = new RealtimeProjectionSyncState();
    sync.beginCatchUp();
    projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) })
    });
    projection.rooms.set('R1', projectedRoom('R1'));
    const navigation = new NavigationStore(projection, sync, {
      roomUnreadCounts: {},
      roomImportantUnreadCounts: {}
    });

    expect(navigation.isInitialLoading).toBe(true);
    expect(navigation.currentUserId).toBeNull();
    expect(navigation.rooms).toEqual([]);

    sync.markCaughtUp('cursor');

    expect(navigation.isInitialLoading).toBe(false);
    expect(navigation.currentUserId).toBe('U1');
    expect(navigation.rooms.map((room) => room.id)).toEqual(['R1']);

    sync.markStale();

    expect(navigation.isInitialLoading).toBe(false);
    expect(navigation.rooms.map((room) => room.id)).toEqual(['R1']);
  });
});
