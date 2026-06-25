import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '$lib/gql/graphql';
import type { RoomMember } from '$lib/mentions';
import { shouldHighlightCurrentUserMention } from './messageMentionHighlight';

function member(id: string, login: string, displayName = login): RoomMember {
  return {
    id,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

const members = [member('user-1', 'alice', 'Alice'), member('user-2', 'bob', 'Bob')];

describe('shouldHighlightCurrentUserMention', () => {
  it('highlights when another user mentions the current user', () => {
    expect(
      shouldHighlightCurrentUserMention({
        actorId: 'user-2',
        body: 'Hey @alice, check this out!',
        currentUserId: 'user-1',
        currentUserLogin: 'alice',
        members
      })
    ).toBe(true);
  });

  it('does not highlight self-authored self mentions', () => {
    expect(
      shouldHighlightCurrentUserMention({
        actorId: 'user-1',
        body: 'Note to myself @alice',
        currentUserId: 'user-1',
        currentUserLogin: 'alice',
        members
      })
    ).toBe(false);
  });

  it('does not highlight invalid mentions', () => {
    expect(
      shouldHighlightCurrentUserMention({
        actorId: 'user-2',
        body: 'Hey @nobody',
        currentUserId: 'user-1',
        currentUserLogin: 'alice',
        members
      })
    ).toBe(false);
  });
});
