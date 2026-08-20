import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { MessageUserInteractionState } from './messageUserInteractions.svelte';

const member = {
  id: 'user-1',
  login: 'ada',
  displayName: 'Ada',
  avatarUrl: '',
  presenceStatus: PresenceStatus.ONLINE
};

describe('MessageUserInteractionState', () => {
  it('prefers the live room member over the event actor snapshot', () => {
    const liveMember = { ...member, displayName: 'Ada Lovelace' };
    const state = new MessageUserInteractionState(() => [liveMember]);

    state.showUser({ ...member, deleted: false }, null);

    expect(state.user).toEqual(liveMember);
  });

  it('retains an actor who is no longer in the room', () => {
    const state = new MessageUserInteractionState(() => []);

    state.showUser({ ...member, deleted: false, isBot: true }, null);

    expect(state.user).toEqual({
      ...member,
      deleted: false,
      isBot: true,
      customStatus: undefined
    });
    expect(state.hasCurrentMember(member.id)).toBe(false);
  });

  it('only opens mention users that are current room members', () => {
    const state = new MessageUserInteractionState(() => [member]);
    const rect = new DOMRect(1, 2, 3, 4);

    state.showMember('missing', rect);
    expect(state.user).toBeNull();

    state.showMember(member.id, rect);
    expect(state.user).toEqual(member);
    expect(state.anchorRect).toBe(rect);

    state.close();
    expect(state.user).toBeNull();
    expect(state.anchorRect).toBeNull();
  });
});
