import { describe, expect, it } from 'vitest';
import { RoomKind } from '$lib/api-client/roomDirectory';
import {
  notificationPolicyCellApplicable,
  notificationPolicyColumns
} from './notificationPolicyMatrix';

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

  it('limits direct-message controls to the server and DM conversations', () => {
    const columns = notificationPolicyColumns('Server', groups, rooms, '');
    const applicability = Object.fromEntries(
      columns.map((column) => [column.key, notificationPolicyCellApplicable('directMessages', column)])
    );

    expect(applicability).toEqual({
      server: true,
      'roomGroup:g1': false,
      'room:r1': false,
      'roomGroup:g2': false,
      'room:r2': false,
      'room:d1': true
    });
    expect(columns.every((column) => notificationPolicyCellApplicable('directMentions', column))).toBe(
      true
    );
  });
});
