import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createNotificationAPI,
  groupNotificationOccurrences,
  NotificationAttentionLevel,
  NotificationPolicyKind,
  type NotificationOccurrenceItem
} from './notifications';

const batchDeleteNotificationOccurrences = vi.hoisted(() => vi.fn());

vi.mock('./connect.js', () => ({
  authHeaders: () => new Headers(),
  createChattoClient: () => ({ batchDeleteNotificationOccurrences })
}));

beforeEach(() => {
  batchDeleteNotificationOccurrences.mockReset();
});

function occurrence(
  id: string,
  reason: NotificationPolicyKind,
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
    signalKind: reason,
    targetSupported: true,
    attentionLevel:
      reason === NotificationPolicyKind.REACTION
        ? NotificationAttentionLevel.AMBIENT
        : NotificationAttentionLevel.IMPORTANT,
    unread: true,
    reactionEmoji: null,
    ...overrides
  };
}

describe('groupNotificationOccurrences', () => {
  it('keeps separate message jump targets separate while grouping direct messages by room', () => {
    const groups = groupNotificationOccurrences([
      occurrence('mention-a', NotificationPolicyKind.DIRECT_MENTION),
      occurrence('mention-b', NotificationPolicyKind.DIRECT_MENTION),
      occurrence('dm-a', NotificationPolicyKind.DIRECT_MESSAGE),
      occurrence('dm-b', NotificationPolicyKind.DIRECT_MESSAGE)
    ]);

    expect(groups).toHaveLength(3);
    expect(groups.find((group) => group.id === 'dm:room')?.occurrences).toHaveLength(2);
  });

  it('consolidates reaction actors and emojis by the reacted-to target', () => {
    const groups = groupNotificationOccurrences([
      occurrence('reaction-a', NotificationPolicyKind.REACTION, {
        eventId: 'message',
        reactionEmoji: '👍'
      }),
      occurrence('reaction-b', NotificationPolicyKind.REACTION, {
        eventId: 'message',
        reactionEmoji: '❤️'
      })
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]?.attentionLevel).toBe(NotificationAttentionLevel.AMBIENT);
    expect(groups[0]?.occurrences.map(({ reactionEmoji }) => reactionEmoji)).toEqual(['👍', '❤️']);
  });

  it('uses the strongest unread attention level in a presentation group', () => {
    const groups = groupNotificationOccurrences([
      occurrence('dm-ambient', NotificationPolicyKind.DIRECT_MESSAGE, {
        attentionLevel: NotificationAttentionLevel.AMBIENT
      }),
      occurrence('dm-important', NotificationPolicyKind.DIRECT_MESSAGE, {
        attentionLevel: NotificationAttentionLevel.IMPORTANT
      })
    ]);

    expect(groups[0]?.attentionLevel).toBe(NotificationAttentionLevel.IMPORTANT);
  });

  it('keeps replies exact even when followed-thread policy also matched', () => {
    const replies = ['reply-a', 'reply-b'].map((id) =>
      occurrence(id, NotificationPolicyKind.REPLY, {
        threadRootId: 'thread'
      })
    );

    expect(groupNotificationOccurrences(replies)).toHaveLength(2);
  });

  it('consolidates a high-cardinality direct-message conversation without losing IDs', () => {
    const occurrences = Array.from({ length: 125 }, (_, index) =>
      occurrence(`dm-${index}`, NotificationPolicyKind.DIRECT_MESSAGE, {
        createdAt: new Date(Date.UTC(2026, 7, 12, 12, 0, index)).toISOString()
      })
    );

    const groups = groupNotificationOccurrences(occurrences);

    expect(groups).toHaveLength(1);
    expect(groups[0]?.occurrences).toHaveLength(125);
    expect(new Set(groups[0]?.occurrences.map(({ id }) => id)).size).toBe(125);
    expect(groups[0]?.openTarget?.id).toBe('dm-124');
  });
});

describe('notification deletion API', () => {
  it('deduplicates and chunks occurrence IDs at the 100-ID request limit', async () => {
    batchDeleteNotificationOccurrences.mockImplementation(
      ({ notificationIds }: { notificationIds: string[] }) =>
        Promise.resolve({ deletedCount: notificationIds.length })
    );
    const ids = [
      ...Array.from({ length: 205 }, (_, index) => `notification-${index}`),
      'notification-0'
    ];

    const deleted = await createNotificationAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    }).batchDeleteNotificationOccurrences(ids);

    expect(deleted).toBe(205);
    expect(batchDeleteNotificationOccurrences).toHaveBeenCalledTimes(3);
    expect(
      batchDeleteNotificationOccurrences.mock.calls.map(
        ([request]) => request.notificationIds.length
      )
    ).toEqual([100, 100, 5]);
    expect(
      batchDeleteNotificationOccurrences.mock.calls.flatMap(([request]) => request.notificationIds)
    ).toEqual(Array.from({ length: 205 }, (_, index) => `notification-${index}`));
  });

  it('stops after a failed deletion chunk', async () => {
    batchDeleteNotificationOccurrences
      .mockResolvedValueOnce({ deletedCount: 100 })
      .mockRejectedValueOnce(new Error('second chunk failed'));

    await expect(
      createNotificationAPI({
        baseUrl: '/api/connect',
        bearerToken: null
      }).batchDeleteNotificationOccurrences(
        Array.from({ length: 205 }, (_, index) => `notification-${index}`)
      )
    ).rejects.toThrow('second chunk failed');

    expect(batchDeleteNotificationOccurrences).toHaveBeenCalledTimes(2);
  });
});
