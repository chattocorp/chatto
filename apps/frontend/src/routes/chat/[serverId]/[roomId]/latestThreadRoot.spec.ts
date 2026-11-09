import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TimelineEventKind, type TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import { latestThreadRootCandidate } from './latestThreadRoot';

function root(
  id: string,
  overrides: {
    createdAt?: string;
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
    createdAt: overrides.createdAt ?? '2026-11-09T12:00:00Z',
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
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-11-09T12:30:00Z'));
  });

  afterEach(() => vi.useRealTimers());

  it('offers the newest root regardless of its author, even without replies', () => {
    const events = [root('mine'), root('someone-else', { actorId: 'other' })];
    expect(latestThreadRootCandidate(events, 'room-1')).toEqual({
      threadRootEventId: 'someone-else'
    });
  });

  it.each([
    ['just posted', '2026-11-09T12:30:00Z', true],
    ['just under 30 minutes old', '2026-11-09T12:00:00.001Z', true],
    ['exactly 30 minutes old', '2026-11-09T12:00:00Z', true],
    ['over 30 minutes old', '2026-11-09T11:59:59.999Z', false],
    ['from the future', '2026-11-09T12:30:00.001Z', false],
    ['invalid', 'invalid', false]
  ])('checks the original posting time when it is %s', (_, createdAt, eligible) => {
    expect(latestThreadRootCandidate([root('thread', { createdAt })], 'room-1')).toEqual(
      eligible ? { threadRootEventId: 'thread' } : null
    );
  });

  it('does not extend the window for edits or new replies', () => {
    const event = root('old-thread', { createdAt: '2026-11-09T11:00:00Z' });
    const activeThread = {
      ...event,
      event: {
        ...event.event,
        updatedAt: '2026-11-09T12:30:00Z',
        lastReplyAt: '2026-11-09T12:30:00Z'
      }
    };
    expect(latestThreadRootCandidate([activeThread], 'room-1')).toBeNull();
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
