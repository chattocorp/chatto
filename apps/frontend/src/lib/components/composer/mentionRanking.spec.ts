import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { describe, expect, it } from 'vitest';

import type { RoomMember } from '$lib/state/room';
import { rankMentionCandidates } from './mentionRanking';

function member(login: string, displayName = login): RoomMember {
  return {
    id: `user_${login}`,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE
  };
}

function handles(...args: Parameters<typeof rankMentionCandidates>): string[] {
  return rankMentionCandidates(...args).map((result) => result.handle);
}

describe('rankMentionCandidates', () => {
  const members = [member('cha'), member('chaz6'), member('chatto'), member('chatto_bot')];

  it('orders users by fuzzy score and handle without prioritized users', () => {
    expect(handles('cha', members, [])).toEqual(['cha', 'chaz6', 'chatto', 'chatto_bot']);
  });

  it('ranks prioritized users above better-scoring users', () => {
    expect(handles('ch', members, [], new Set(['user_chatto_bot']))).toEqual([
      'chatto_bot',
      'cha',
      'chaz6',
      'chatto'
    ]);
  });

  it('keeps an exact match above prioritized users', () => {
    expect(handles('cha', members, [], new Set(['user_chatto_bot']))).toEqual([
      'cha',
      'chatto_bot',
      'chaz6',
      'chatto'
    ]);
  });

  it('does not treat a long fuzzy match that scores above 1000 as an exact match', () => {
    const query = 'a'.repeat(26) + 'x';
    const scattered = member('a'.repeat(26) + '_x');
    const prioritized = member('spaced', 'a '.repeat(26) + 'x');

    expect(handles(query, [scattered, prioritized], [], new Set([prioritized.id]))).toEqual([
      'spaced',
      scattered.login
    ]);
  });

  it('keeps users above virtual handles and roles, even when prioritized', () => {
    const roles = [{ name: 'helpers', pingable: true }];
    expect(
      handles('he', [member('heidi'), member('hector')], roles, new Set(['user_heidi']))
    ).toEqual(['heidi', 'hector', 'here', 'helpers']);
  });
});
