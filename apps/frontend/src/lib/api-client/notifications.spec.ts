import { describe, expect, it } from 'vitest';
import { NotificationReason } from './notifications';
import { groupNotificationOccurrences, type NotificationOccurrenceItem } from './notifications';

function occurrence(
  id: string,
  reason: NotificationReason,
  overrides: Partial<NotificationOccurrenceItem> = {}
): NotificationOccurrenceItem {
  return {
    id,
    sourceEventId: `source-${id}`,
    createdAt: `2026-08-12T12:00:0${id.length}Z`,
    actor: null,
    room: { id: 'room', name: 'Room' },
    eventId: `event-${id}`,
    threadRootId: null,
    parentEventId: null,
    reasons: [reason],
    reasonMatches: [],
    unread: true,
    reactionEmoji: null,
    threadRootMessageExcerpt: null,
    ...overrides
  };
}

describe('groupNotificationOccurrences', () => {
  it('keeps separate message jump targets separate while grouping direct messages by room', () => {
    const groups = groupNotificationOccurrences([
      occurrence('mention-a', NotificationReason.DIRECT_MENTION),
      occurrence('mention-b', NotificationReason.DIRECT_MENTION, {
        reasons: [NotificationReason.DIRECT_MENTION, NotificationReason.FOLLOWED_ROOM]
      }),
      occurrence('dm-a', NotificationReason.DIRECT_MESSAGE),
      occurrence('dm-b', NotificationReason.DIRECT_MESSAGE)
    ]);

    expect(groups).toHaveLength(3);
    expect(groups.find((group) => group.id === 'dm:room')?.occurrences).toHaveLength(2);
  });

  it('consolidates reaction actors and emojis by the reacted-to target', () => {
    const groups = groupNotificationOccurrences([
      occurrence('reaction-a', NotificationReason.REACTION, {
        eventId: 'message',
        reactionEmoji: '👍'
      }),
      occurrence('reaction-b', NotificationReason.REACTION, {
        eventId: 'message',
        reactionEmoji: '❤️'
      })
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]?.occurrences.map(({ reactionEmoji }) => reactionEmoji)).toEqual(['👍', '❤️']);
  });

  it('keeps replies exact even when followed-thread policy also matched', () => {
    const replies = ['reply-a', 'reply-b'].map((id) =>
      occurrence(id, NotificationReason.REPLY, {
        reasons: [NotificationReason.REPLY, NotificationReason.FOLLOWED_THREAD],
        threadRootId: 'thread'
      })
    );

    expect(groupNotificationOccurrences(replies)).toHaveLength(2);
  });
});
