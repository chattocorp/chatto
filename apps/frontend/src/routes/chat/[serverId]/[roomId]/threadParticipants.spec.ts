import { describe, expect, it } from 'vitest';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { threadParticipantIds } from './threadParticipants';

function message(id: string, actorId: string | null, threadParticipants: string[] = []) {
  return {
    id,
    createdAt: '2026-09-27T12:00:00Z',
    actorId,
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room_test',
      body: 'hello',
      attachments: [],
      reactions: [],
      replyCount: 0,
      threadParticipants: threadParticipants.map((participantId) => ({ id: participantId }))
    }
  } as unknown as TimelineEventView;
}

describe('threadParticipantIds', () => {
  it('collects reply authors and the root participant list without duplicates', () => {
    const ids = threadParticipantIds(
      [
        message('root', 'user_root', ['user_old_reply', 'user_reply']),
        message('reply_1', 'user_reply'),
        message('reply_2', 'user_reply')
      ],
      'root',
      null
    );

    expect([...ids].sort()).toEqual(['user_old_reply', 'user_reply', 'user_root']);
  });

  it('excludes the viewer', () => {
    const ids = threadParticipantIds(
      [message('root', 'user_viewer', ['user_viewer', 'user_bot']), message('r', 'user_viewer')],
      'root',
      'user_viewer'
    );

    expect([...ids]).toEqual(['user_bot']);
  });

  it('ignores participant lists on replies, events without an actor, and non-message events', () => {
    const systemEvent = {
      id: 'joined',
      createdAt: '2026-09-27T12:00:00Z',
      actorId: 'user_joiner',
      event: { kind: TimelineEventKind.UserJoinedRoom, roomId: 'room_test' }
    } as TimelineEventView;

    const ids = threadParticipantIds(
      [message('root', null), message('reply', 'user_reply', ['user_other']), systemEvent],
      'root',
      null
    );

    expect([...ids]).toEqual(['user_reply']);
  });
});
