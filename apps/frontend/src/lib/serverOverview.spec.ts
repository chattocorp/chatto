import { describe, expect, it } from 'vitest';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { RoomsListItem } from '$lib/state/server/navigation';
import { catchUpRooms, missingProfileItems } from './serverOverview';

function listed(id: string, overrides: Partial<RoomsListItem> = {}): RoomsListItem {
  return {
    id,
    name: id,
    type: RoomKind.CHANNEL,
    isUniversal: false,
    viewerIsMember: true,
    viewerCanJoinRoom: true,
    viewerCanManageRoom: false,
    viewerNotificationCount: 0,
    viewerImportantNotificationCount: 0,
    members: [],
    ...overrides
  };
}

describe('missingProfileItems', () => {
  it('lists empty profile fields in settings order', () => {
    expect(missingProfileItems({ description: '  ' })).toEqual([
      'logo',
      'banner',
      'description',
      'welcome_message'
    ]);
  });

  it('returns nothing for a complete profile', () => {
    expect(
      missingProfileItems({
        logoUrl: '/logo.png',
        bannerUrl: '/banner.png',
        description: 'A server',
        welcomeMessage: 'Hello'
      })
    ).toEqual([]);
  });
});

describe('catchUpRooms', () => {
  it('lists joined channel rooms with activity, important notifications first', () => {
    const unread = new Set(['quiet']);
    const rooms = [
      listed('quiet'),
      listed('idle'),
      listed('busy', { viewerNotificationCount: 3 }),
      listed('mention', { viewerNotificationCount: 2, viewerImportantNotificationCount: 1 }),
      listed('dm', { type: RoomKind.DM, viewerNotificationCount: 5 }),
      listed('other', { viewerIsMember: false, viewerNotificationCount: 1 })
    ];

    expect(catchUpRooms(rooms, (id) => unread.has(id))).toEqual([
      { id: 'mention', name: 'mention', importantCount: 1, otherCount: 1 },
      { id: 'busy', name: 'busy', importantCount: 0, otherCount: 3 },
      { id: 'quiet', name: 'quiet', importantCount: 0, otherCount: 0 }
    ]);
  });

  it('keeps at most the limit', () => {
    const rooms = ['a', 'b', 'c'].map((id) => listed(id, { viewerNotificationCount: 1 }));
    expect(catchUpRooms(rooms, () => false, 2).map((room) => room.id)).toEqual(['a', 'b']);
  });
});
