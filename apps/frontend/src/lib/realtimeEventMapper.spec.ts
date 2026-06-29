import { Timestamp } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import { realtimeEventToEventEnvelope } from '$lib/realtimeEventMapper';
import { RoomEventKind } from '$lib/render/eventKinds';
import {
  RealtimeEventEnvelope,
  RealtimeMentionNotificationEvent,
  RealtimeNewDirectMessageNotificationEvent
} from '$lib/pb/chatto/realtime/v1/realtime_pb';

describe('realtimeEventToEventEnvelope', () => {
  it('preserves mention notification display data', () => {
    const event = realtimeEventToEventEnvelope(
      new RealtimeEventEnvelope({
        id: 'evt-mention',
        createdAt: Timestamp.now(),
        actorId: 'user-1',
        event: {
          case: 'mentionNotification',
          value: new RealtimeMentionNotificationEvent({
            roomId: 'room-1',
            actorUserId: 'user-1',
            roomName: 'General',
            actorDisplayName: 'Ada Lovelace'
          })
        }
      })
    ) as unknown as {
      event: {
        kind: string;
        room: { name: string };
        actor: { id: string; displayName: string } | null;
      };
    };

    expect(event.event.kind).toBe(RoomEventKind.MentionNotification);
    expect(event.event.room.name).toBe('General');
    expect(event.event.actor).toEqual({
      id: 'user-1',
      displayName: 'Ada Lovelace'
    });
  });

  it('preserves DM notification display data', () => {
    const event = realtimeEventToEventEnvelope(
      new RealtimeEventEnvelope({
        id: 'evt-dm',
        createdAt: Timestamp.now(),
        actorId: 'user-2',
        event: {
          case: 'newDirectMessageNotification',
          value: new RealtimeNewDirectMessageNotificationEvent({
            roomId: 'dm-1',
            senderId: 'user-2',
            senderDisplayName: 'Grace Hopper',
            senderAvatarUrl: '/assets/avatar.png',
            conversationName: 'Grace Hopper'
          })
        }
      })
    ) as unknown as {
      event: {
        kind: string;
        conversationName: string;
        sender: { id: string; displayName: string; avatarUrl: string | null } | null;
      };
    };

    expect(event.event.kind).toBe(RoomEventKind.NewDirectMessageNotification);
    expect(event.event.conversationName).toBe('Grace Hopper');
    expect(event.event.sender).toEqual({
      id: 'user-2',
      displayName: 'Grace Hopper',
      avatarUrl: '/assets/avatar.png'
    });
  });
});
