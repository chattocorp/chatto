import { describe, expect, it } from 'vitest';
import { mergeQuickSwitcherUserResults, type QuickSwitcherItemBase } from './quickSwitcherItems';

type Item = QuickSwitcherItemBase & {
  id: string;
};

const currentUserId = 'user-self';

function dm(id: string, serverId: string, participantIds: string[]): Item {
  return {
    kind: 'dm',
    id,
    serverId,
    currentUserId,
    participants: participantIds.map((participantId) => ({ id: participantId }))
  };
}

function user(id: string, serverId: string): Item {
  return {
    kind: 'user',
    id,
    serverId,
    targetUserId: id
  };
}

describe('mergeQuickSwitcherUserResults', () => {
  it('does not change base items when there are no user results', () => {
    const base = [dm('dm-a', 'server-a', [currentUserId, 'user-a'])];

    expect(mergeQuickSwitcherUserResults(base, [])).toEqual(base);
  });

  it('suppresses matching one-to-one DM results on the same server', () => {
    const room = { kind: 'room' as const, id: 'room-a', serverId: 'server-a' };
    const base = [room, dm('dm-a', 'server-a', [currentUserId, 'user-a'])];

    expect(mergeQuickSwitcherUserResults(base, [user('user-a', 'server-a')])).toEqual([
      room,
      user('user-a', 'server-a')
    ]);
  });

  it('keeps matching one-to-one DM results from another server', () => {
    const serverADm = dm('dm-a', 'server-a', [currentUserId, 'user-a']);
    const serverBDm = dm('dm-b', 'server-b', [currentUserId, 'user-a']);

    expect(
      mergeQuickSwitcherUserResults([serverADm, serverBDm], [user('user-a', 'server-a')])
    ).toEqual([serverBDm, user('user-a', 'server-a')]);
  });

  it('suppresses matching self-DM results', () => {
    expect(
      mergeQuickSwitcherUserResults(
        [dm('dm-self', 'server-a', [currentUserId])],
        [user(currentUserId, 'server-a')]
      )
    ).toEqual([user(currentUserId, 'server-a')]);
  });

  it('preserves group DM results', () => {
    const groupDm = dm('dm-group', 'server-a', [currentUserId, 'user-a', 'user-b']);

    expect(mergeQuickSwitcherUserResults([groupDm], [user('user-a', 'server-a')])).toEqual([
      groupDm,
      user('user-a', 'server-a')
    ]);
  });
});
