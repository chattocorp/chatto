import { describe, expect, it } from 'vitest';
import { RoomType } from '$lib/render/types';
import type { RoomsListItem } from '$lib/state/server/rooms.svelte';
import { roomLinkAccessRedirect } from './roomLinkAccess';

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

describe('roomLinkAccessRedirect', () => {
  it('allows access when the viewer is already a member', () => {
    expect(
      roomLinkAccessRedirect({
        rooms: [room()],
        roomId: 'room-1',
        targetPath: '/chat/-/room-1',
        fallbackPath: '/chat/-'
      })
    ).toEqual({ kind: 'allow' });
  });

  it('allows unknown rooms to fall through to room data loading', () => {
    expect(
      roomLinkAccessRedirect({
        rooms: [],
        roomId: 'room-1',
        targetPath: '/chat/-/room-1',
        fallbackPath: '/chat/-'
      })
    ).toEqual({ kind: 'allow' });
  });

  it('redirects nonmember rooms with a join modal and original target', () => {
    expect(
      roomLinkAccessRedirect({
        rooms: [room({ viewerIsMember: false })],
        roomId: 'room-1',
        targetPath: '/chat/-/room-1/m/message-1',
        fallbackPath: '/chat/-'
      })
    ).toEqual({
      kind: 'redirect',
      path: '/chat/-',
      state: {
        modal: {
          type: 'joinRoom',
          roomId: 'room-1',
          roomName: 'general',
          viewerCanJoinRoom: true,
          afterJoinPath: '/chat/-/room-1/m/message-1',
          closePath: '/chat/-'
        }
      }
    });
  });

  it('uses the same modal shape for non-joinable rooms', () => {
    expect(
      roomLinkAccessRedirect({
        rooms: [room({ viewerIsMember: false, viewerCanJoinRoom: false })],
        roomId: 'room-1',
        targetPath: '/chat/-/room-1',
        fallbackPath: '/chat/-'
      })
    ).toMatchObject({
      kind: 'redirect',
      state: {
        modal: {
          type: 'joinRoom',
          viewerCanJoinRoom: false
        }
      }
    });
  });
});
