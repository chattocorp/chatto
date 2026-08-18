import { describe, expect, it } from 'vitest';
import {
  ListNotificationOccurrencesResponse,
  NotificationOccurrence,
  NotificationRoomUnreadCount
} from '@chatto/api-types/api/v1/notifications_pb';

import {
  notificationOccurrence,
  mapNotificationOccurrencePage,
  NotificationAttentionLevel,
  NotificationDeliveryIntensity,
  NotificationPolicyKind
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

  it('keeps followed-thread targets intact', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'thread-notification',
        sourceEventId: 'reply-1',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('followedThreadActivity', 'reply-1', 'root-1'),
        intensity: NotificationDeliveryIntensity.BADGE,
        unread: true
      })
    );

    expect(occurrence).toMatchObject({
      signalKind: NotificationPolicyKind.FOLLOWED_THREAD,
      eventId: 'reply-1',
      threadRootId: 'root-1'
    });
  });

  it('maps a direct mention as its own exact signal', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'thread-mention',
        sourceEventId: 'reply-2',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('directMentionReceived', 'reply-2', 'root-1'),
        intensity: NotificationDeliveryIntensity.ALERT,
        unread: true
      })
    );

    expect(occurrence.signalKind).toBe(NotificationPolicyKind.DIRECT_MENTION);
    expect(occurrence.attentionLevel).toBe(NotificationAttentionLevel.IMPORTANT);
    expect(occurrence).toMatchObject({
      eventId: 'reply-2',
      threadRootId: 'root-1'
    });
  });

  it('describes followed-room occurrences as messages', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'room-notification',
        sourceEventId: 'message-1',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('followedRoomActivity', 'message-1'),
        intensity: NotificationDeliveryIntensity.ALERT,
        unread: true
      })
    );

    expect(occurrence).toMatchObject({
      signalKind: NotificationPolicyKind.FOLLOWED_ROOM,
      eventId: 'message-1'
    });
  });

  it('preserves a threaded reaction target', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'reaction-notification',
        sourceEventId: 'reaction-1',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('reactionReceived', 'message-1', 'thread-root-1', 'heart'),
        intensity: NotificationDeliveryIntensity.BADGE,
        unread: true
      })
    );

    expect(occurrence).toMatchObject({
      signalKind: NotificationPolicyKind.REACTION,
      eventId: 'message-1',
      threadRootId: 'thread-root-1'
    });
    expect(occurrence.attentionLevel).toBe(NotificationAttentionLevel.AMBIENT);
  });

  it('keeps unsupported targets as safe generic rows with authoritative counts', () => {
    const page = mapNotificationOccurrencePage(
      new ListNotificationOccurrencesResponse({
        unreadCount: 1,
        importantUnreadCount: 1,
        occurrences: [
          new NotificationOccurrence({
            id: 'future-target',
            sourceEventId: 'future-source',
            signal: { kind: { case: undefined } },
            unread: true
          })
        ]
      })
    );

    expect(page.occurrences).toHaveLength(1);
    expect(page.occurrences[0]).toMatchObject({
      id: 'future-target',
      targetSupported: false,
      room: null,
      eventId: '',
      unread: true
    });
    expect(page.unreadCount).toBe(1);
    expect(page.importantUnreadCount).toBe(1);
    expect(page.consumedCount).toBe(1);
  });
});

function notificationSignal(
  kind:
    | 'directMentionReceived'
    | 'followedThreadActivity'
    | 'followedRoomActivity'
    | 'reactionReceived',
  eventId: string,
  threadRootEventId?: string,
  emoji?: string
) {
  const message = {
    room: { id: 'room-1', name: 'general' },
    eventId,
    threadRootEventId
  };
  return {
    kind: {
      case: kind,
      value: kind === 'reactionReceived' ? { message, emoji } : { message }
    }
  };
}

function requireNotificationOccurrence(item: NotificationOccurrence) {
  return notificationOccurrence(item);
}
