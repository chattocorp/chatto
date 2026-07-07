import { describe, expect, it } from 'vitest';
import { RoomType } from '$lib/render/types';
import type { RoomsListItem } from '$lib/state/server/rooms.svelte';
import { roomRouteAccess } from './roomLinkAccess';

function room(overrides: Partial<RoomsListItem> = {}): RoomsListItem {
  return {
    id: 'room-1',
    name: 'general',
    type: RoomType.Channel,
    isUniversal: false,
    hasUnread: false,
    viewerIsMember: true,
    viewerCanJoinRoom: true,
    viewerNotificationCount: 0,
    members: [],
    ...overrides
  };
}

describe('roomRouteAccess', () => {
  it('classifies members as allowed room viewers', () => {
    expect(
      roomRouteAccess({
        rooms: [room()],
        roomId: 'room-1'
      })
    ).toEqual({ kind: 'member' });
  });

  it('allows unknown rooms to fall through to room data loading', () => {
    expect(
      roomRouteAccess({
        rooms: [],
        roomId: 'room-1'
      })
    ).toEqual({ kind: 'unknown' });
  });

  it('classifies joinable nonmember rooms for inline joining', () => {
    expect(
      roomRouteAccess({
        rooms: [room({ viewerIsMember: false })],
        roomId: 'room-1'
      })
    ).toEqual({
      kind: 'nonmember',
      room: room({ viewerIsMember: false })
    });
  });

  it('classifies restricted nonmember rooms for inline access denial', () => {
    expect(
      roomRouteAccess({
        rooms: [room({ viewerIsMember: false, viewerCanJoinRoom: false })],
        roomId: 'room-1'
      })
    ).toMatchObject({
      kind: 'nonmember',
      room: {
        viewerCanJoinRoom: false
      }
    });
  });
});
