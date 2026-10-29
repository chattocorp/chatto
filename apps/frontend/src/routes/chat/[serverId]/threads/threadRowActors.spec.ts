import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { UserAvatarUserView } from '@chatto/client/timeline/users';
import { threadRowActors } from './threadRowActors';

const user = (id: string): UserAvatarUserView => ({
  id,
  login: id,
  displayName: id,
  presenceStatus: PresenceStatus.OFFLINE,
  customStatus: null
});

describe('threadRowActors', () => {
  it('shows the root author even when participants are in first-reply order', () => {
    const [alice, bob, carol] = [user('alice'), user('bob'), user('carol')];
    // Bob replied first, Carol replied last.
    expect(threadRowActors(alice, [bob, carol], carol)).toEqual([alice, bob]);
  });

  it('falls back to the latest reply author for the badge', () => {
    const [alice, bob] = [user('alice'), user('bob')];
    expect(threadRowActors(bob, [bob], alice)).toEqual([bob, alice]);
  });

  it('shows one avatar when only one person is known', () => {
    const alice = user('alice');
    expect(threadRowActors(alice, [], alice)).toEqual([alice]);
  });

  it('uses the other candidates when the root author is unknown', () => {
    const [alice, bob] = [user('alice'), user('bob')];
    expect(threadRowActors(null, [bob], alice)).toEqual([bob, alice]);
    expect(threadRowActors(null, [], null)).toEqual([]);
  });
});
