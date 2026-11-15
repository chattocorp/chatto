import { describe, expect, it } from 'vitest';
import { PermissionGrant } from '@chatto/api-types/api/v1/permissions_pb';
import {
  RoomGroup,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { Room, RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { ServerProjectionStore } from '@chatto/client/server/projection';
import { RealtimeProjectionSyncState } from '@chatto/client/server/realtimeSync';
import { RoomListView } from '@chatto/client/server/rooms';
import { isNavigationVisibleRoom, NavigationStore } from './navigation';

function projectedRoom(
  id: string,
  options: { kind?: RoomKind; hasMessageHistory?: boolean; canReadMessages?: boolean } = {}
): RoomWithViewerState {
  return new RoomWithViewerState({
    room: new Room({ id, name: id, kind: options.kind ?? RoomKind.CHANNEL }),
    viewerState: new RoomViewerState({
      isMember: true,
      permissions: [
        new PermissionGrant({
          permission: 'message.read',
          granted: options.canReadMessages ?? true
        })
      ]
    }),
    hasMessageHistory: options.hasMessageHistory
  });
}

function navigationFor(
  projection: ServerProjectionStore,
  counts: {
    roomUnreadCounts: Record<string, number>;
    roomImportantUnreadCounts: Record<string, number>;
  }
) {
  const sync = new RealtimeProjectionSyncState();
  sync.markCaughtUp(undefined);
  return new NavigationStore(new RoomListView(projection, sync), () => ({
    unreadNotificationCount: 0,
    importantUnreadNotificationCount: 0,
    ...counts
  }));
}

describe('NavigationStore', () => {
  it('keeps management metadata in the client while hiding it from chat navigation', () => {
    const projection = new ServerProjectionStore();
    const visible = projectedRoom('visible');
    const hidden = new RoomWithViewerState({
      room: { id: 'hidden', name: 'Management only', kind: RoomKind.CHANNEL },
      viewerState: {
        isMember: false,
        permissions: [
          { permission: 'room.list', granted: false },
          { permission: 'room.manage', granted: true }
        ]
      }
    });
    projection.rooms.set('visible', visible);
    projection.rooms.set('hidden', hidden);
    projection.roomGroups = [
      new RoomGroup({
        id: 'g1',
        name: 'Group',
        items: [
          { item: { case: 'room', value: visible } },
          { item: { case: 'room', value: hidden } },
          { item: { case: 'sidebarLink', value: { id: 'docs', label: 'Docs', url: '/docs' } } }
        ]
      })
    ];
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp(undefined);
    const roomList = new RoomListView(projection, sync);
    const navigation = navigationFor(projection, {
      roomUnreadCounts: {},
      roomImportantUnreadCounts: {}
    });
    expect(roomList.rooms.map((room) => room.id)).toEqual(['visible', 'hidden']);
    expect(roomList.roomGroups[0]?.roomIds).toEqual(['visible', 'hidden']);
    expect(navigation.rooms.map((room) => room.id)).toEqual(['visible']);
    expect(navigation.roomGroups[0]?.roomIds).toEqual(['visible']);
    expect(navigation.roomGroups[0]?.items?.map((item) => item.id)).toEqual([
      'room:visible',
      'link:docs'
    ]);
    const nowVisible = hidden.clone();
    nowVisible.viewerState!.permissions[0]!.granted = true;
    projection.rooms.set('hidden', nowVisible);
    projection.roomGroups = [
      new RoomGroup({
        ...projection.roomGroups[0],
        items: [
          { item: { case: 'room', value: visible } },
          { item: { case: 'room', value: nowVisible } }
        ]
      })
    ];
    expect(navigation.rooms.map((room) => room.id)).toEqual(['visible', 'hidden']);
    expect(navigation.roomGroups[0]?.roomIds).toEqual(['visible', 'hidden']);
  });

  it('adds the notification counts of each room', () => {
    const projection = new ServerProjectionStore();
    projection.rooms.set('dm', projectedRoom('dm', { kind: RoomKind.DM, hasMessageHistory: true }));
    projection.rooms.set('ambient', projectedRoom('ambient'));
    const navigation = navigationFor(projection, {
      roomUnreadCounts: { dm: 3, ambient: 1 },
      roomImportantUnreadCounts: { dm: 2 }
    });

    expect(navigation.rooms).toMatchObject([
      { id: 'dm', viewerNotificationCount: 3, viewerImportantNotificationCount: 2 },
      // A missing Important count is zero after the last Important occurrence is read.
      { id: 'ambient', viewerNotificationCount: 1, viewerImportantNotificationCount: 0 }
    ]);
    expect(navigation.isRoomMember('dm')).toBe(true);
  });

  it('uses DM history instead of message.read to show a direct message', () => {
    const projection = new ServerProjectionStore();
    projection.rooms.set(
      'unreadable-dm',
      projectedRoom('unreadable-dm', {
        kind: RoomKind.DM,
        hasMessageHistory: true,
        canReadMessages: false
      })
    );
    projection.rooms.set(
      'empty-dm',
      projectedRoom('empty-dm', { kind: RoomKind.DM, hasMessageHistory: false })
    );
    const navigation = navigationFor(projection, {
      roomUnreadCounts: {},
      roomImportantUnreadCounts: {}
    });

    expect(navigation.rooms.filter(isNavigationVisibleRoom).map((room) => room.id)).toEqual([
      'unreadable-dm'
    ]);
  });
});
