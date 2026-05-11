import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { computeEventMetadata } from './messageGrouping';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { UserSettingsState } from '$lib/state/userSettings.svelte';

// Mock settings with explicit UTC timezone so tests are deterministic regardless of host TZ
const defaultSettings = {
  get effectiveTimezone(): string | undefined {
    return 'UTC';
  },
  get effectiveHour12(): boolean | undefined {
    return undefined;
  }
} as unknown as UserSettingsState;

function createMockEvent(
  overrides: Partial<{
    id: string;
    actorId: string;
    createdAt: string;
    typename: 'MessagePostedEvent' | 'UserJoinedRoomEvent' | 'UserLeftRoomEvent';
    body: string | null;
    attachments: unknown[];
  }> = {}
): RoomEventViewFragment {
  const typename = overrides.typename ?? 'MessagePostedEvent';

  const baseEvent = {
    id: overrides.id ?? `evt_${Math.random().toString(36).slice(2)}`,
    createdAt: overrides.createdAt ?? new Date().toISOString(),
    actorId: overrides.actorId ?? 'u_user1',
    actor: {
      id: overrides.actorId ?? 'u_user1',
      login: 'testuser',
      avatarUrl: null
    }
  };

  if (typename === 'MessagePostedEvent') {
    return {
      ...baseEvent,
      event: {
        __typename: 'MessagePostedEvent',
        roomId: 'r_test',

        body: 'body' in overrides ? overrides.body : 'Test message',
        attachments: overrides.attachments ?? [],
        linkPreview: null,
        reactions: [],
        updatedAt: null,
        inReplyTo: null,
        inThread: null,
        replyCount: 0,
        lastReplyAt: null,
        threadParticipants: [],
        viewerIsFollowingThread: null
      }
    } as RoomEventViewFragment;
  }

  return {
    ...baseEvent,
    event: {
      __typename: typename,
      roomId: 'r_test'
    }
  } as RoomEventViewFragment;
}

describe('computeEventMetadata', () => {
  beforeEach(() => {
    // Mock Date to control "today" for day label tests
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-11-28T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('empty and single event cases', () => {
    it('returns empty array for empty input', () => {
      expect(computeEventMetadata([], defaultSettings)).toEqual([]);
    });

    it('marks single event as first in group with day separator', () => {
      const event = createMockEvent({ createdAt: '2025-11-28T10:00:00Z' });
      const result = computeEventMetadata([event], defaultSettings);

      expect(result).toHaveLength(1);
      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[0].showDaySeparator).toBe(true);
      expect(result[0].dayLabel).toBe('Today');
    });
  });

  describe('message grouping', () => {
    it('groups consecutive messages from same user within 10 minutes', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:05:00Z'
        }),
        createMockEvent({
          id: 'evt_3',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:09:00Z'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(false);
      expect(result[2].isFirstInGroup).toBe(false);
    });

    it('starts new group when more than 10 minutes apart', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:11:00Z'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(true);
    });

    it('starts new group when different user', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_bob',
          createdAt: '2025-11-28T10:01:00Z'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(true);
    });

    it('does not group system events with messages', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z',
          typename: 'MessagePostedEvent'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:01:00Z',
          typename: 'UserJoinedRoomEvent'
        }),
        createMockEvent({
          id: 'evt_3',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:02:00Z',
          typename: 'MessagePostedEvent'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(true);
      expect(result[2].isFirstInGroup).toBe(true);
    });

    it('starts new group for reply messages even from same user', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:01:00Z'
        }),
        createMockEvent({
          id: 'evt_3',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:02:00Z'
        })
      ];

      // Make evt_3 a reply
      const replyEvent = events[2].event as { inReplyTo: string | null };
      replyEvent.inReplyTo = 'evt_other';

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(false); // normal grouping
      expect(result[2].isFirstInGroup).toBe(true); // reply breaks the group
    });

    it('skips hidden deleted messages for grouping (next message becomes first in group)', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z',
          body: null // deleted, no attachments/reactions/replies → hidden
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:01:00Z',
          body: 'Still here'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(true); // hidden prev → starts new group
    });

    it('groups with visible deleted message that has reactions', () => {
      const deletedWithReactions = createMockEvent({
        id: 'evt_1',
        actorId: 'u_alice',
        createdAt: '2025-11-28T10:00:00Z',
        body: null
      });
      // Add a reaction so it's visible (not hidden)
      (deletedWithReactions.event as Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>).reactions = [
        { emoji: '👍', count: 1, hasReacted: false, users: [] }
      ];

      const events = [
        deletedWithReactions,
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:01:00Z',
          body: 'After deleted'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(false); // visible deleted msg → groups normally
    });

    it('groups with visible deleted message that has replies', () => {
      const deletedWithReplies = createMockEvent({
        id: 'evt_1',
        actorId: 'u_alice',
        createdAt: '2025-11-28T10:00:00Z',
        body: null
      });
      (deletedWithReplies.event as Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>).replyCount = 3;

      const events = [
        deletedWithReplies,
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:01:00Z',
          body: 'After deleted with thread'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(false); // visible (has replies) → groups normally
    });
  });

  describe('day separators', () => {
    it('shows day separator for first message', () => {
      const event = createMockEvent({ createdAt: '2025-11-28T10:00:00Z' });
      const result = computeEventMetadata([event], defaultSettings);

      expect(result[0].showDaySeparator).toBe(true);
    });

    it('shows day separator when day changes', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          createdAt: '2025-11-27T23:59:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          createdAt: '2025-11-28T00:01:00Z'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].showDaySeparator).toBe(true);
      expect(result[1].showDaySeparator).toBe(true);
    });

    it('does not show day separator for same day messages', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-28T10:00:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_bob',
          createdAt: '2025-11-28T22:00:00Z'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].showDaySeparator).toBe(true);
      expect(result[1].showDaySeparator).toBe(false);
    });

    it('shows day separator on visible message after hidden ones from previous day', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-27T23:59:00Z',
          body: null // hidden deleted message from yesterday
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T00:01:00Z',
          body: 'First visible message today'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      // Hidden event still gets showDaySeparator (it won't be rendered by virtualItems)
      expect(result[0].showDaySeparator).toBe(true);
      // Visible event also gets showDaySeparator because its visible prevEvent is null
      expect(result[1].showDaySeparator).toBe(true);
    });

    it('starts new group when day changes even if same user within 10 mins', () => {
      const events = [
        createMockEvent({
          id: 'evt_1',
          actorId: 'u_alice',
          createdAt: '2025-11-27T23:58:00Z'
        }),
        createMockEvent({
          id: 'evt_2',
          actorId: 'u_alice',
          createdAt: '2025-11-28T00:02:00Z'
        })
      ];

      const result = computeEventMetadata(events, defaultSettings);

      expect(result[0].isFirstInGroup).toBe(true);
      expect(result[1].isFirstInGroup).toBe(true);
      expect(result[1].showDaySeparator).toBe(true);
    });
  });

  describe('day labels', () => {
    it('labels today as "Today"', () => {
      const event = createMockEvent({ createdAt: '2025-11-28T10:00:00Z' });
      const result = computeEventMetadata([event], defaultSettings);

      expect(result[0].dayLabel).toBe('Today');
    });

    it('labels yesterday as "Yesterday"', () => {
      const event = createMockEvent({ createdAt: '2025-11-27T10:00:00Z' });
      const result = computeEventMetadata([event], defaultSettings);

      expect(result[0].dayLabel).toBe('Yesterday');
    });

    it('uses full date format for older dates', () => {
      const event = createMockEvent({ createdAt: '2025-11-20T10:00:00Z' });
      const result = computeEventMetadata([event], defaultSettings);

      expect(result[0].dayLabel).toMatch(/Thursday, November 20/);
    });

    it('includes year for dates from different year', () => {
      const event = createMockEvent({ createdAt: '2024-12-25T10:00:00Z' });
      const result = computeEventMetadata([event], defaultSettings);

      expect(result[0].dayLabel).toMatch(/2024/);
    });
  });
});
