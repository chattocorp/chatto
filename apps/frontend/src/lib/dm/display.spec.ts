import { describe, expect, it } from 'vitest';
import {
  getDMAvatarParticipants,
  getDMConversationLabel,
  hasVisibleDMParticipant,
  type DMDisplayParticipant
} from './display';

const deletedUserLabel = 'Deleted User';

function participant(id: string, displayName: string, deleted = false): DMDisplayParticipant {
  return {
    id,
    login: displayName.toLowerCase().replace(/\s+/g, '-'),
    displayName,
    deleted
  };
}

describe('DM display helpers', () => {
  it('uses the other participant for a normal 1:1 DM', () => {
    expect(
      getDMConversationLabel(
        [participant('current', 'Current User'), participant('peer', 'Peer User')],
        'current',
        deletedUserLabel
      )
    ).toBe('Peer User');
  });

  it('uses the deleted-user label when the only listed DM member is the current user', () => {
    expect(
      getDMConversationLabel([participant('current', 'Current User')], 'current', deletedUserLabel)
    ).toBe(deletedUserLabel);
  });

  it('joins non-current participants for group DMs', () => {
    expect(
      getDMConversationLabel(
        [
          participant('current', 'Current User'),
          participant('peer-a', 'Peer A'),
          participant('peer-b', 'Peer B')
        ],
        'current',
        deletedUserLabel
      )
    ).toBe('Peer A, Peer B');
  });

  it('keeps deleted non-current participants labeled as deleted users', () => {
    expect(
      getDMConversationLabel(
        [participant('current', 'Current User'), participant('peer', 'Former Peer', true)],
        'current',
        deletedUserLabel
      )
    ).toBe(deletedUserLabel);
  });

  it('keeps avatar participants aligned with the visible DM label participants', () => {
    const current = participant('current', 'Current User');
    const peerA = participant('peer-a', 'Peer A');
    const peerB = participant('peer-b', 'Peer B');

    expect(getDMAvatarParticipants([current, peerA, peerB], 'current', 1)).toEqual([peerA]);
    expect(getDMAvatarParticipants([current], 'current', 1)).toEqual([current]);
  });

  it('detects whether a DM has a visible non-current participant', () => {
    expect(
      hasVisibleDMParticipant(
        [participant('current', 'Current User'), participant('peer', 'Peer User')],
        'current'
      )
    ).toBe(true);
    expect(hasVisibleDMParticipant([participant('current', 'Current User')], 'current')).toBe(
      false
    );
    expect(
      hasVisibleDMParticipant(
        [participant('current', 'Current User'), participant('peer', 'Former Peer', true)],
        'current'
      )
    ).toBe(false);
  });
});
