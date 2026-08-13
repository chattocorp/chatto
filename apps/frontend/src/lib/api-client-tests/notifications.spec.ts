import { describe, expect, it } from 'vitest';
import {
  ListNotificationOccurrencesResponse,
  NotificationOccurrence,
  NotificationRoomUnreadCount
} from '@chatto/api-types/api/v1/notifications_pb';

import {
  notificationOccurrence,
  mapNotificationOccurrencePage,
  occurrenceAsNotificationItem,
  NotificationAttentionLevel,
  NotificationDeliveryIntensity,
  NotificationItemKind,
  NotificationReason
} from '$lib/api-client/notifications';

describe('notification occurrence presentation mapping', () => {
  it('distinguishes absent attention counts from current zero counts', () => {
    const legacy = mapNotificationOccurrencePage(
      new ListNotificationOccurrencesResponse({
        unreadCount: 3,
        roomUnreadCounts: [new NotificationRoomUnreadCount({ roomId: 'room-1', unreadCount: 2 })]
      })
    );
    const current = mapNotificationOccurrencePage(
      new ListNotificationOccurrencesResponse({
        unreadCount: 3,
        importantUnreadCount: 0,
        roomUnreadCounts: [
          new NotificationRoomUnreadCount({
            roomId: 'room-1',
            unreadCount: 2,
            importantUnreadCount: 0
          })
        ]
      })
    );

    expect(legacy.importantUnreadCount).toBe(3);
    expect(legacy.roomImportantUnreadCounts).toEqual({ 'room-1': 2 });
    expect(current.importantUnreadCount).toBe(0);
    expect(current.roomImportantUnreadCounts).toEqual({ 'room-1': 0 });
  });

  it('keeps followed-thread targets as reply notifications', () => {
    const occurrence = notificationOccurrence(
      new NotificationOccurrence({
        id: 'thread-notification',
        sourceEventId: 'reply-1',
        actor: { id: 'u1', displayName: 'Alice' },
        target: {
          room: { id: 'room-1', name: 'general' },
          eventId: 'reply-1',
          threadRootEventId: 'root-1'
        },
        reasons: [
          {
            reason: NotificationReason.FOLLOWED_THREAD,
            intensity: NotificationDeliveryIntensity.BADGE
          }
        ],
        unread: true
      })
    );

    expect(occurrence.reasons).toEqual([NotificationReason.FOLLOWED_THREAD]);
    expect(occurrenceAsNotificationItem(occurrence)).toMatchObject({
      kind: NotificationItemKind.Reply,
      replyEventId: 'reply-1',
      replyInThread: 'root-1'
    });
  });

  it('keeps a direct mention ahead of badge-only followed-thread activity', () => {
    const occurrence = notificationOccurrence(
      new NotificationOccurrence({
        id: 'thread-mention',
        sourceEventId: 'reply-2',
        actor: { id: 'u1', displayName: 'Alice' },
        target: {
          room: { id: 'room-1', name: 'general' },
          eventId: 'reply-2',
          threadRootEventId: 'root-1'
        },
        reasons: [
          {
            reason: NotificationReason.DIRECT_MENTION,
            intensity: NotificationDeliveryIntensity.ALERT
          },
          {
            reason: NotificationReason.FOLLOWED_THREAD,
            intensity: NotificationDeliveryIntensity.BADGE
          }
        ],
        unread: true
      })
    );

    expect(occurrence.reasons).toEqual([
      NotificationReason.DIRECT_MENTION,
      NotificationReason.FOLLOWED_THREAD
    ]);
    expect(occurrence.attentionLevel).toBe(NotificationAttentionLevel.IMPORTANT);
    expect(occurrenceAsNotificationItem(occurrence)).toMatchObject({
      kind: NotificationItemKind.Mention,
      mentionEventId: 'reply-2',
      mentionInThread: 'root-1'
    });
  });

  it('describes followed-room occurrences as messages', () => {
    const occurrence = notificationOccurrence(
      new NotificationOccurrence({
        id: 'room-notification',
        sourceEventId: 'message-1',
        actor: { id: 'u1', displayName: 'Alice' },
        target: { room: { id: 'room-1', name: 'general' }, eventId: 'message-1' },
        reasons: [
          {
            reason: NotificationReason.FOLLOWED_ROOM,
            intensity: NotificationDeliveryIntensity.ALERT
          }
        ],
        unread: true
      })
    );

    expect(occurrence.reasons).toEqual([NotificationReason.FOLLOWED_ROOM]);
    expect(occurrenceAsNotificationItem(occurrence)).toMatchObject({
      kind: NotificationItemKind.RoomMessage,
      roomMsgEventId: 'message-1'
    });
  });

  it('preserves a threaded reaction target in the flattened room-message shape', () => {
    const occurrence = notificationOccurrence(
      new NotificationOccurrence({
        id: 'reaction-notification',
        sourceEventId: 'reaction-1',
        actor: { id: 'u1', displayName: 'Alice' },
        target: {
          room: { id: 'room-1', name: 'general' },
          eventId: 'message-1',
          threadRootEventId: 'thread-root-1'
        },
        reasons: [
          {
            reason: NotificationReason.REACTION,
            intensity: NotificationDeliveryIntensity.BADGE
          }
        ],
        unread: true
      })
    );

    expect(occurrenceAsNotificationItem(occurrence)).toMatchObject({
      kind: NotificationItemKind.RoomMessage,
      roomMsgEventId: 'message-1',
      roomMsgThreadRootId: 'thread-root-1'
    });
    expect(occurrence.attentionLevel).toBe(NotificationAttentionLevel.AMBIENT);
  });
});
