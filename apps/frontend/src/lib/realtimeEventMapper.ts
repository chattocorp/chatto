import { PresenceStatus as GqlPresenceStatus } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { RealtimeEventEnvelope } from '@chatto/api-types/realtime/v1/realtime_pb';
import { PresenceStatus as ApiPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { EventEnvelope } from '$lib/eventBus.svelte';

function timestampToISO(value: { toDate(): Date } | undefined): string {
  return value?.toDate().toISOString() ?? new Date().toISOString();
}

function presenceStatus(status: ApiPresenceStatus): GqlPresenceStatus {
  switch (status) {
    case ApiPresenceStatus.AWAY:
      return GqlPresenceStatus.Away;
    case ApiPresenceStatus.DO_NOT_DISTURB:
      return GqlPresenceStatus.DoNotDisturb;
    case ApiPresenceStatus.ONLINE:
      return GqlPresenceStatus.Online;
    case ApiPresenceStatus.OFFLINE:
    case ApiPresenceStatus.UNSPECIFIED:
    default:
      return GqlPresenceStatus.Offline;
  }
}

export function realtimeEventToEventEnvelope(frame: RealtimeEventEnvelope): EventEnvelope | null {
  const base = {
    id: frame.id,
    createdAt: timestampToISO(frame.createdAt),
    actorId: frame.actorId ?? null
  };

  switch (frame.event.case) {
    case 'userTyping': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserTyping,
          roomId: value.roomId,
          typingThreadRootEventId: value.threadRootEventId ?? null
        }
      };
    }
    case 'presenceChanged':
      return {
        ...base,
        actorId: frame.event.value.userId || base.actorId,
        event: {
          kind: RoomEventKind.PresenceChanged,
          status: presenceStatus(frame.event.value.status)
        }
      };
    case 'mentionNotification': {
      const value = frame.event.value;
      return {
        ...base,
        actorId: value.actorUserId || base.actorId,
        event: {
          kind: RoomEventKind.MentionNotification,
          roomId: value.roomId,
          actorUserId: value.actorUserId,
          actorDisplayName: value.actorDisplayName ?? 'Unknown user',
          roomName: value.roomName ?? ''
        }
      };
    }
    case 'newDirectMessageNotification': {
      const value = frame.event.value;
      return {
        ...base,
        actorId: value.senderId || base.actorId,
        event: {
          kind: RoomEventKind.NewDirectMessageNotification,
          roomId: value.roomId,
          senderId: value.senderId,
          senderDisplayName: value.senderDisplayName ?? 'Unknown user',
          senderAvatarUrl: value.senderAvatarUrl ?? '',
          conversationName: value.conversationName ?? ''
        }
      };
    }
    case 'sessionTerminated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.SessionTerminated,
          reason: frame.event.value.reason
        }
      };
    default:
      return null;
  }
}
