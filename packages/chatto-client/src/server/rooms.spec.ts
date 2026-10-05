import { describe, expect, it } from 'vitest';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { PermissionGrant } from '@chatto/api-types/api/v1/permissions_pb';
import {
  RoomGroup,
  RoomGroupItem,
  RoomGroupViewerState,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { Room, RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { GetViewerResponse, ViewerUser } from '@chatto/api-types/api/v1/viewer_pb';
import { ServerProjectionStore } from './projection.js';
import { RealtimeProjectionSyncState } from './realtimeSync.js';
import { RoomListView } from './rooms.js';

function roomListFor(
  projection: ServerProjectionStore,
  sync = new RealtimeProjectionSyncState()
): { roomList: RoomListView; sync: RealtimeProjectionSyncState } {
  sync.markCaughtUp(undefined);
  return { roomList: new RoomListView(projection, sync), sync };
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
): RoomWithViewerState {
  return new RoomWithViewerState({
    room: new Room({ id, name: id, kind }),
    viewerState: new RoomViewerState({
      isMember: member,
      permissions: [
        new PermissionGrant({ permission: 'room.join', granted: true }),
        new PermissionGrant({ permission: 'room.manage', granted: id === 'managed' }),
        new PermissionGrant({ permission: 'message.read', granted: canReadMessages })
      ]
    }),
    memberUserIds,
    hasMessageHistory
  });
}

describe('RoomListView', () => {
  it('resolves a deleted DM participant to a deleted placeholder', () => {
    const projection = new ServerProjectionStore();
    projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) })
    });
    projection.users.set(
      'gone',
      new DirectoryMember({ user: new User({ id: 'gone', deleted: true }) })
    );
    projection.rooms.set(
      'dm',
      projectedRoom('dm', {
        kind: RoomKind.DM,
        memberUserIds: ['U1', 'gone', 'pending'],
        hasMessageHistory: true
      })
    );

    const { roomList } = roomListFor(projection);
    const members = roomList.rooms.find((room) => room.id === 'dm')?.members ?? [];

    // The viewer and the pending profile are unresolved; the deleted one is a placeholder.
    expect(members).toEqual([expect.objectContaining({ id: 'gone', deleted: true })]);
  });

  it('selects rooms, members, permissions, and viewer identity from the projection', () => {
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

    const { roomList } = roomListFor(projection);

    expect(roomList.viewerId).toBe('U1');
    expect(roomList.isLoading).toBe(false);
    expect(roomList.isMember('managed')).toBe(true);
    expect(roomList.rooms).toMatchObject([
      {
        id: 'dm',
        type: RoomKind.DM,
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
        viewerState: new RoomGroupViewerState({
          permissions: [new PermissionGrant({ permission: 'room.create', granted: true })]
        }),
        items: [
          new RoomGroupItem({
            item: { case: 'room', value: projectedRoom('newer') }
          })
        ]
      })
    ];
    const { roomList } = roomListFor(projection);

    expect(roomList.rooms.map((room) => room.id)).toEqual(['older', 'newer']);
    expect(roomList.roomGroups).toMatchObject([
      { id: 'G1', name: 'Projects', roomIds: ['newer'], viewerCanCreateRoom: true }
    ]);

    projection.rooms.delete('older');
    expect(roomList.rooms.map((room) => room.id)).toEqual(['newer']);
  });

  it('becomes empty immediately when the canonical projection resets', () => {
    const projection = new ServerProjectionStore();
    projection.viewer = new GetViewerResponse();
    projection.rooms.set('R1', projectedRoom('R1'));
    const { roomList, sync } = roomListFor(projection);

    projection.reset();
    sync.acceptProjectionEvent(undefined, true);

    expect(roomList.rooms).toEqual([]);
    expect(roomList.roomGroups).toEqual([]);
    expect(roomList.isLoading).toBe(true);
  });

  it('hides a snapshot prefix until caught up while retaining stale state', () => {
    const projection = new ServerProjectionStore();
    const sync = new RealtimeProjectionSyncState();
    sync.beginCatchUp();
    projection.viewer = new GetViewerResponse({
      user: new ViewerUser({ profile: new User({ id: 'U1' }) })
    });
    projection.rooms.set('R1', projectedRoom('R1'));
    const roomList = new RoomListView(projection, sync);

    expect(roomList.isLoading).toBe(true);
    expect(roomList.viewerId).toBeNull();
    expect(roomList.rooms).toEqual([]);

    sync.markCaughtUp('cursor');

    expect(roomList.isLoading).toBe(false);
    expect(roomList.viewerId).toBe('U1');
    expect(roomList.rooms.map((room) => room.id)).toEqual(['R1']);

    sync.markStale();

    expect(roomList.isLoading).toBe(false);
    expect(roomList.rooms.map((room) => room.id)).toEqual(['R1']);
  });
});
