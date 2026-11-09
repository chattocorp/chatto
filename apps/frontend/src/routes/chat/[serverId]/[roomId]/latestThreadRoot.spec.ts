import { describe, expect, it } from 'vitest';
import { TimelineEventKind, type TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import { latestThreadRootCandidate } from './latestThreadRoot';

function root(
  id: string,
  overrides: {
    actorId?: string;
    roomId?: string;
    threadExists?: boolean;
    deletedAt?: string | null;
    threadRootEventId?: string;
  } = {}
): TimelineEventView {
  return {
    id,
    actorId: overrides.actorId ?? 'me',
    actor: null,
    createdAt: '2020-01-01T00:00:00Z',
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: overrides.roomId ?? 'room-1',
      body: id,
      attachments: [],
      reactions: [],
      threadExists: overrides.threadExists ?? true,
      replyCount: 0,
      threadParticipants: [],
      deletedAt: overrides.deletedAt ?? null,
      threadRootEventId: overrides.threadRootEventId
    }
  };
}

describe('latestThreadRootCandidate', () => {
  it('offers the newest root regardless of its author or age, even without replies', () => {
    const events = [root('mine'), root('someone-else', { actorId: 'other' })];
    expect(latestThreadRootCandidate(events, 'room-1')).toEqual({
      threadRootEventId: 'someone-else'
    });
  });

  it('does not skip a newer flat root to find an older thread', () => {
    expect(
      latestThreadRootCandidate([root('thread'), root('flat', { threadExists: false })], 'room-1')
    ).toBeNull();
  });

  it('does not skip a deleted latest root to find an older thread', () => {
    expect(
      latestThreadRootCandidate(
        [root('thread'), root('deleted', { deletedAt: '2026-11-09T12:00:00Z' })],
        'room-1'
      )
    ).toBeNull();
  });

  it('ignores other rooms and thread replies', () => {
    expect(
      latestThreadRootCandidate(
        [
          root('thread'),
          root('foreign', { roomId: 'room-2' }),
          root('reply', { threadRootEventId: 'thread' })
        ],
        'room-1'
      )
    ).toEqual({ threadRootEventId: 'thread' });
  });

  it('has no candidate without a room or root', () => {
    expect(latestThreadRootCandidate([], 'room-1')).toBeNull();
    expect(latestThreadRootCandidate([root('thread')], '')).toBeNull();
  });
});
