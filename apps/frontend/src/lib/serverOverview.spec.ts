import { describe, expect, it } from 'vitest';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { RoomsListItem } from '$lib/state/server/navigation';
import {
  catchUpRooms,
  isNewRoom,
  missingProfileItems,
  recentDirectMessages
} from './serverOverview';

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

describe('isNewRoom', () => {
  const now = Date.parse('2026-11-26T12:00:00Z');

  it('treats rooms from the last seven days as new', () => {
    expect(isNewRoom('2026-11-20T12:00:01Z', now)).toBe(true);
    expect(isNewRoom('2026-11-19T12:00:00Z', now)).toBe(false);
  });

  it('treats unknown or invalid creation times as not new', () => {
    expect(isNewRoom(null, now)).toBe(false);
    expect(isNewRoom('not a date', now)).toBe(false);
  });
});

describe('recentDirectMessages', () => {
  it('lists direct messages with history, newest first, up to the limit', () => {
    const rooms = [
      listed('channel'),
      listed('old', { type: RoomKind.DM, lastMessageAt: '2026-11-01T00:00:00Z' }),
      listed('new', { type: RoomKind.DM, lastMessageAt: '2026-11-25T00:00:00Z' }),
      listed('empty', { type: RoomKind.DM, hasMessageHistory: false }),
      listed('unknown', { type: RoomKind.DM }),
      listed('mid', { type: RoomKind.DM, lastMessageAt: '2026-11-10T00:00:00Z' })
    ];

    expect(recentDirectMessages(rooms, 3).map((room) => room.id)).toEqual(['new', 'mid', 'old']);
  });
});
