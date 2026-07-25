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
import { NavigationStore } from './rooms.svelte';

function projectedRoom(
  id: string,
  {
    kind = RoomKind.CHANNEL,
    member = true,
    count = 0,
    memberUserIds = [],
    hasMessageHistory
  }: {
    kind?: RoomKind;
    member?: boolean;
    count?: number;
    memberUserIds?: string[];
    hasMessageHistory?: boolean;
  } = {}
): RealtimeProjectionRoom {
  return new RealtimeProjectionRoom({
    room: new RoomWithViewerState({
      room: new Room({ id, name: id, kind }),
      viewerState: new RoomViewerState({
        isMember: member,
        permissions: [
          new PermissionGrant({ permission: 'room.join', granted: true }),
          new PermissionGrant({ permission: 'room.manage', granted: id === 'managed' })
        ]
      })
    }),
    memberUserIds,
    viewerNotificationCount: count,
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
        count: 3,
        memberUserIds: ['U2'],
        hasMessageHistory: true
      })
    );
    projection.rooms.set('managed', projectedRoom('managed'));

    const navigation = new NavigationStore(projection);

    expect(navigation.currentUserId).toBe('U1');
    expect(navigation.isInitialLoading).toBe(false);
    expect(navigation.rooms).toMatchObject([
      {
        id: 'dm',
        type: RoomKind.DM,
        viewerNotificationCount: 3,
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
    const navigation = new NavigationStore(projection);

    expect(navigation.rooms.map((room) => room.id)).toEqual(['older', 'newer']);
    expect(navigation.roomGroups).toMatchObject([
      { id: 'G1', name: 'Projects', roomIds: ['newer'] }
    ]);

    projection.rooms.delete('older');
    expect(navigation.rooms.map((room) => room.id)).toEqual(['newer']);
  });

  it('becomes empty immediately when the canonical projection resets', () => {
    const projection = new ServerProjectionStore();
    projection.viewer = new GetViewerResponse();
    projection.rooms.set('R1', projectedRoom('R1'));
    const navigation = new NavigationStore(projection);

    projection.reset();

    expect(navigation.rooms).toEqual([]);
    expect(navigation.roomGroups).toEqual([]);
    expect(navigation.isInitialLoading).toBe(true);
  });
});
