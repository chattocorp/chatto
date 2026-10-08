import { describe, expect, it } from 'vitest';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { RoomsListItem } from '$lib/state/server/navigation';
import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms';
import {
  homeDirectMessages,
  homeDirectMessagesLoading,
  homeLiveCalls,
  type HomeServerSource
} from './homeOverview';

function room(id: string, overrides: Partial<RoomsListItem> = {}): RoomsListItem {
  return {
    id,
    name: id,
    type: RoomKind.DM,
    isUniversal: false,
    viewerIsMember: true,
    viewerCanJoinRoom: false,
    viewerCanManageRoom: false,
    hasMessageHistory: true,
    members: [],
    viewerNotificationCount: 0,
    viewerImportantNotificationCount: 0,
    ...overrides
  };
}

const alice: CallRoomParticipant = {
  userId: 'alice',
  displayName: 'Alice',
  login: 'alice',
  avatarUrl: null,
  isBot: false
};

function source(overrides: Partial<HomeServerSource> = {}): HomeServerSource {
  return {
    serverId: 'server-a',
    serverName: 'Server A',
    viewerId: 'viewer',
    rooms: [],
    roomsLoading: false,
    isUnread: () => false,
    callRoomIds: [],
    callParticipants: () => [],
    ...overrides
  };
}

describe('homeDirectMessages', () => {
  it('lists the visible direct messages of every server', () => {
    const result = homeDirectMessages([
      source({
        rooms: [
          room('dm-a'),
          room('general', { type: RoomKind.CHANNEL }),
          room('dm-empty', { hasMessageHistory: false })
        ]
      }),
      source({ serverId: 'server-b', serverName: 'Server B', rooms: [room('dm-b')] })
    ]);

    expect(result.map((entry) => [entry.serverId, entry.room.id])).toEqual([
      ['server-a', 'dm-a'],
      ['server-b', 'dm-b']
    ]);
    expect(result[1]).toMatchObject({ serverName: 'Server B', viewerId: 'viewer' });
  });

  it('puts important notifications, then notifications, then unread messages first', () => {
    const result = homeDirectMessages([
      source({
        rooms: [
          room('quiet'),
          room('unread'),
          room('ambient', { viewerNotificationCount: 3 }),
          room('important', { viewerNotificationCount: 1, viewerImportantNotificationCount: 1 })
        ],
        isUnread: (roomId) => roomId === 'unread' || roomId === 'ambient'
      })
    ]);

    expect(result.map((entry) => entry.room.id)).toEqual([
      'important',
      'ambient',
      'unread',
      'quiet'
    ]);
    expect(result.find((entry) => entry.room.id === 'unread')?.unread).toBe(true);
  });

  it('marks direct messages with a call in progress', () => {
    const [entry] = homeDirectMessages([source({ rooms: [room('dm-a')], callRoomIds: ['dm-a'] })]);

    expect(entry.hasActiveCall).toBe(true);
  });
});

describe('homeLiveCalls', () => {
  it('lists calls in direct messages and joined rooms only', () => {
    const result = homeLiveCalls([
      source({
        rooms: [
          room('dm-a'),
          room('joined', { type: RoomKind.CHANNEL }),
          room('not-joined', { type: RoomKind.CHANNEL, viewerIsMember: false })
        ],
        callRoomIds: ['dm-a', 'joined', 'not-joined', 'unknown'],
        callParticipants: (roomId) => (roomId === 'joined' ? [alice] : [])
      })
    ]);

    expect(result.map((call) => call.room.id)).toEqual(['dm-a', 'joined']);
    expect(result[1].participants).toEqual([alice]);
  });
});

describe('homeDirectMessagesLoading', () => {
  it('loads until one server can list its rooms', () => {
    expect(homeDirectMessagesLoading([])).toBe(false);
    expect(homeDirectMessagesLoading([source({ roomsLoading: true })])).toBe(true);
    expect(
      homeDirectMessagesLoading([
        source({ roomsLoading: true }),
        source({ serverId: 'server-b', roomsLoading: false })
      ])
    ).toBe(false);
  });
});
