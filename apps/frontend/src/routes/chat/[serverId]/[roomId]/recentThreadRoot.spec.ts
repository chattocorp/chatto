import { describe, expect, it } from 'vitest';
import {
  TimelineEventKind,
  type TimelineEventView
} from '$lib/render/timelineEvents';
import {
  RECENT_THREAD_ROOT_WINDOW_MS,
  recentThreadRootCandidate
} from './recentThreadRoot';

const now = Date.parse('2026-08-23T12:00:00Z');

function root(
  id: string,
  overrides: {
    actorId?: string;
    roomId?: string;
    age?: number;
    threadExists?: boolean;
    deletedAt?: string | null;
  } = {}
): TimelineEventView {
  return {
    id,
    actorId: overrides.actorId ?? 'me',
    actor: null,
    createdAt: new Date(now - (overrides.age ?? 0)).toISOString(),
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: overrides.roomId ?? 'room-1',
      body: id,
      attachments: [],
      reactions: [],
      threadExists: overrides.threadExists ?? true,
      replyCount: 0,
      threadParticipants: [],
      deletedAt: overrides.deletedAt ?? null
    }
  };
}

describe('recentThreadRootCandidate', () => {
  it('selects the latest authored threaded root through the five-minute boundary', () => {
    const events = [
      root('mine', { age: RECENT_THREAD_ROOT_WINDOW_MS }),
      root('someone-else', { actorId: 'other' })
    ];

    expect(recentThreadRootCandidate(events, 'room-1', 'me', now)).toEqual({
      threadRootEventId: 'mine'
    });
  });

  it('rejects the candidate just after the five-minute boundary', () => {
    expect(
      recentThreadRootCandidate(
        [root('mine', { age: RECENT_THREAD_ROOT_WINDOW_MS + 1 })],
        'room-1',
        'me',
        now
      )
    ).toBeNull();
  });

  it('does not skip a newer flat root to find an older threaded root', () => {
    const events = [root('older-thread'), root('newer-flat', { threadExists: false })];
    expect(recentThreadRootCandidate(events, 'room-1', 'me', now)).toBeNull();
  });

  it('rejects deleted, cross-room, future, and malformed candidates', () => {
    expect(
      recentThreadRootCandidate(
        [root('deleted', { deletedAt: new Date(now).toISOString() })],
        'room-1',
        'me',
        now
      )
    ).toBeNull();
    expect(
      recentThreadRootCandidate([root('foreign', { roomId: 'room-2' })], 'room-1', 'me', now)
    ).toBeNull();
    expect(
      recentThreadRootCandidate([root('future', { age: -1 })], 'room-1', 'me', now)
    ).toBeNull();
    expect(
      recentThreadRootCandidate(
        [{ ...root('invalid'), createdAt: 'not-a-date' }],
        'room-1',
        'me',
        now
      )
    ).toBeNull();
  });
});
