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
  NotificationSignalKind
} from '$lib/api-client/notifications';

describe('notification occurrence presentation mapping', () => {
  it('preserves authoritative attention counts', () => {
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

    expect(current.importantUnreadCount).toBe(0);
    expect(current.roomImportantUnreadCounts).toEqual({ 'room-1': 0 });
  });

  it('keeps followed-thread targets intact', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'thread-notification',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('followedThreadActivity', 'reply-1', 'root-1'),
        attentionLevel: NotificationAttentionLevel.IMPORTANT,
        unread: true
      })
    );

    expect(occurrence).toMatchObject({
      signalKind: NotificationSignalKind.FOLLOWED_THREAD,
      eventId: 'reply-1',
      threadRootId: 'root-1'
    });
  });

  it('maps a direct mention as its own exact signal', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'thread-mention',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('directMentionReceived', 'reply-2', 'root-1'),
        attentionLevel: NotificationAttentionLevel.IMPORTANT,
        unread: true
      })
    );

    expect(occurrence.signalKind).toBe(NotificationSignalKind.DIRECT_MENTION);
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
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('followedRoomActivity', 'message-1'),
        attentionLevel: NotificationAttentionLevel.IMPORTANT,
        unread: true
      })
    );

    expect(occurrence).toMatchObject({
      signalKind: NotificationSignalKind.FOLLOWED_ROOM,
      eventId: 'message-1'
    });
  });

  it('preserves a threaded reaction target', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'reaction-notification',
        actor: { id: 'u1', displayName: 'Alice' },
        signal: notificationSignal('reactionReceived', 'message-1', 'thread-root-1', 'heart'),
        attentionLevel: NotificationAttentionLevel.AMBIENT,
        unread: true
      })
    );

    expect(occurrence).toMatchObject({
      signalKind: NotificationSignalKind.REACTION,
      eventId: 'message-1',
      threadRootId: 'thread-root-1'
    });
    expect(occurrence.attentionLevel).toBe(NotificationAttentionLevel.AMBIENT);
  });

  it('maps unknown future attention levels conservatively to Important', () => {
    const occurrence = requireNotificationOccurrence(
      new NotificationOccurrence({
        id: 'future-attention',
        signal: notificationSignal('directMentionReceived', 'message-1'),
        attentionLevel: 99 as NotificationAttentionLevel,
        unread: true
      })
    );

    expect(occurrence.attentionLevel).toBe(NotificationAttentionLevel.IMPORTANT);
  });

  it('keeps unsupported targets as safe generic rows with authoritative counts', () => {
    const page = mapNotificationOccurrencePage(
      new ListNotificationOccurrencesResponse({
        unreadCount: 1,
        importantUnreadCount: 1,
        occurrences: [
          new NotificationOccurrence({
            id: 'future-target',
            signal: { kind: { case: undefined } },
            attentionLevel: NotificationAttentionLevel.IMPORTANT,
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
