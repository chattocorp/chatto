import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createNotificationAPI,
  groupNotificationOccurrences,
  NotificationAttentionLevel,
  NotificationDeliveryMode,
  NotificationSignalKind,
  type NotificationOccurrenceItem
} from './notifications';

const batchDeleteNotificationOccurrences = vi.hoisted(() => vi.fn());
const getNotificationPolicy = vi.hoisted(() => vi.fn());
const updateNotificationPolicy = vi.hoisted(() => vi.fn());

vi.mock('./connect.js', () => ({
  authHeaders: () => new Headers(),
  createChattoClient: () => ({
    batchDeleteNotificationOccurrences,
    getNotificationPolicy,
    updateNotificationPolicy
  })
}));

beforeEach(() => {
  batchDeleteNotificationOccurrences.mockReset();
  getNotificationPolicy.mockReset();
  updateNotificationPolicy.mockReset();
});

function policyResponse() {
  return {
    policy: {
      overrides: {
        directMessages: NotificationDeliveryMode.ALERT,
        followedRooms: NotificationDeliveryMode.SILENT
      },
      effective: {
        directMessages: NotificationDeliveryMode.ALERT,
        directMentions: NotificationDeliveryMode.ALERT,
        replies: NotificationDeliveryMode.ALERT,
        roleMentions: NotificationDeliveryMode.ALERT,
        hereMentions: NotificationDeliveryMode.ALERT,
        allMentions: NotificationDeliveryMode.ALERT,
        followedThreads: NotificationDeliveryMode.SILENT,
        followedRooms: NotificationDeliveryMode.SILENT,
        reactions: NotificationDeliveryMode.SILENT
      }
    }
  };
}

function occurrence(
  id: string,
  reason: NotificationSignalKind,
  overrides: Partial<NotificationOccurrenceItem> = {}
): NotificationOccurrenceItem {
  return {
    id,
    createdAt: `2026-08-12T12:00:0${id.length}Z`,
    actor: null,
    room: { id: 'room', name: 'Room' },
    eventId: `event-${id}`,
    threadRootId: null,
    signalKind: reason,
    targetSupported: true,
    attentionLevel:
      reason === NotificationSignalKind.REACTION
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
      occurrence('mention-a', NotificationSignalKind.DIRECT_MENTION),
      occurrence('mention-b', NotificationSignalKind.DIRECT_MENTION),
      occurrence('dm-a', NotificationSignalKind.DIRECT_MESSAGE),
      occurrence('dm-b', NotificationSignalKind.DIRECT_MESSAGE)
    ]);

    expect(groups).toHaveLength(3);
    expect(groups.find((group) => group.id === 'dm:room')?.occurrences).toHaveLength(2);
  });

  it('consolidates reaction actors and emojis by the reacted-to target', () => {
    const groups = groupNotificationOccurrences([
      occurrence('reaction-a', NotificationSignalKind.REACTION, {
        eventId: 'message',
        reactionEmoji: '👍'
      }),
      occurrence('reaction-b', NotificationSignalKind.REACTION, {
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
      occurrence('dm-ambient', NotificationSignalKind.DIRECT_MESSAGE, {
        attentionLevel: NotificationAttentionLevel.AMBIENT
      }),
      occurrence('dm-important', NotificationSignalKind.DIRECT_MESSAGE, {
        attentionLevel: NotificationAttentionLevel.IMPORTANT
      })
    ]);

    expect(groups[0]?.attentionLevel).toBe(NotificationAttentionLevel.IMPORTANT);
  });

  it('keeps replies exact even when followed-thread policy also matched', () => {
    const replies = ['reply-a', 'reply-b'].map((id) =>
      occurrence(id, NotificationSignalKind.REPLY, {
        threadRootId: 'thread'
      })
    );

    expect(groupNotificationOccurrences(replies)).toHaveLength(2);
  });

  it('consolidates a high-cardinality direct-message conversation without losing IDs', () => {
    const occurrences = Array.from({ length: 125 }, (_, index) =>
      occurrence(`dm-${index}`, NotificationSignalKind.DIRECT_MESSAGE, {
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

describe('notification policy API', () => {
  it('normalizes explicit policy fields and absent overrides', async () => {
    getNotificationPolicy.mockResolvedValue(policyResponse());

    const policy = await createNotificationAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    }).getNotificationPolicy('room-1');

    expect(getNotificationPolicy).toHaveBeenCalledWith(
      { roomId: 'room-1' },
      { headers: expect.any(Headers) }
    );
    expect(policy.overrides).toMatchObject({
      directMessages: NotificationDeliveryMode.ALERT,
      directMentions: null,
      followedRooms: NotificationDeliveryMode.SILENT,
      reactions: null
    });
    expect(policy.effective.reactions).toBe(NotificationDeliveryMode.SILENT);
  });

  it('sends exact field-mask paths and omits cleared override values', async () => {
    updateNotificationPolicy.mockResolvedValue(policyResponse());

    await createNotificationAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    }).updateNotificationPolicy(
      {
        directMessages: NotificationDeliveryMode.OFF,
        reactions: null
      },
      'room-1'
    );

    expect(updateNotificationPolicy).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        overrides: { directMessages: NotificationDeliveryMode.OFF },
        updateMask: { paths: ['direct_messages', 'reactions'] }
      },
      { headers: expect.any(Headers) }
    );
  });

  it('rejects an empty policy patch without issuing a request', async () => {
    await expect(
      createNotificationAPI({
        baseUrl: '/api/connect',
        bearerToken: null
      }).updateNotificationPolicy({})
    ).rejects.toThrow('Notification policy update is empty');

    expect(updateNotificationPolicy).not.toHaveBeenCalled();
  });
});
