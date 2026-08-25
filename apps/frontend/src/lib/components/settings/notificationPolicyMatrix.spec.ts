import { describe, expect, it } from 'vitest';
import { RoomKind } from '$lib/api-client/roomDirectory';
import { notificationPolicyColumns } from './notificationPolicyMatrix';

const rooms = [
  { id: 'r1', name: 'general', viewerIsMember: true, type: RoomKind.CHANNEL },
  { id: 'r2', name: 'support', viewerIsMember: true, type: RoomKind.CHANNEL },
  { id: 'r3', name: 'hidden', viewerIsMember: false, type: RoomKind.CHANNEL },
  { id: 'd1', name: 'Morgan', viewerIsMember: true, type: RoomKind.DM }
] as Parameters<typeof notificationPolicyColumns>[2];
const groups = [
  { id: 'g1', name: 'Community', roomIds: ['r1', 'r3'] },
  { id: 'g2', name: 'Help', roomIds: ['r2'] }
] as Parameters<typeof notificationPolicyColumns>[1];

describe('notificationPolicyColumns', () => {
  it('orders server, groups with member channels, ungrouped channels, then DMs', () => {
    expect(
      notificationPolicyColumns('Server', groups, rooms, '').map((column) => column.key)
    ).toEqual(['server', 'roomGroup:g1', 'room:r1', 'roomGroup:g2', 'room:r2', 'room:d1']);
  });

  it('retains the parent for a room match and all children for a group match', () => {
    expect(
      notificationPolicyColumns('Server', groups, rooms, 'general').map((column) => column.key)
    ).toEqual(['server', 'roomGroup:g1', 'room:r1']);
    expect(
      notificationPolicyColumns('Server', groups, rooms, 'help').map((column) => column.key)
    ).toEqual(['server', 'roomGroup:g2', 'room:r2']);
  });
});
