import { describe, expect, it } from 'vitest';
import { NotificationOccurrence } from '@chatto/api-types/api/v1/notifications_pb';

import {
  notificationOccurrence,
  occurrenceAsNotificationItem,
  NotificationDeliveryIntensity,
  NotificationItemKind,
  NotificationReason
} from '$lib/api-client/notifications';

describe('notification occurrence presentation mapping', () => {
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

  it('keeps a direct mention ahead of ambient followed-thread activity', () => {
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
  });
});
