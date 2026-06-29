import {
  NotificationLevel as GqlNotificationLevel,
  PresenceStatus as GqlPresenceStatus,
  TimeFormat
} from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { NotificationLevel as ApiNotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import {
  RealtimeEventEnvelope,
  RealtimeHeartbeat
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { PresenceStatus as ApiPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat as ApiTimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import type { EventEnvelope } from '$lib/eventBus.svelte';

function timestampToISO(value: { toDate(): Date } | undefined): string {
  return value?.toDate().toISOString() ?? new Date().toISOString();
}

function optionalTimestampToISO(value: { toDate(): Date } | undefined): string | null {
  return value ? timestampToISO(value) : null;
}

function notificationLevel(level: ApiNotificationLevel): GqlNotificationLevel {
  switch (level) {
    case ApiNotificationLevel.MUTED:
      return GqlNotificationLevel.Muted;
    case ApiNotificationLevel.ALL_MESSAGES:
      return GqlNotificationLevel.AllMessages;
    case ApiNotificationLevel.DEFAULT:
    case ApiNotificationLevel.UNSPECIFIED:
      return GqlNotificationLevel.Default;
    case ApiNotificationLevel.NORMAL:
    default:
      return GqlNotificationLevel.Normal;
  }
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

function timeFormat(format: ApiTimeFormat): TimeFormat {
  switch (format) {
    case ApiTimeFormat.TIME_FORMAT_12_HOUR:
      return TimeFormat.TwelveHour;
    case ApiTimeFormat.TIME_FORMAT_24_HOUR:
      return TimeFormat.TwentyFourHour;
    case ApiTimeFormat.TIME_FORMAT_AUTO:
    case ApiTimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}

export function realtimeHeartbeatToEventEnvelope(frame: RealtimeHeartbeat): EventEnvelope {
  return {
    id: frame.id,
    createdAt: timestampToISO(frame.createdAt),
    actorId: null,
    actor: null,
    event: { kind: RoomEventKind.Heartbeat, alive: true }
  } as unknown as EventEnvelope;
}

export function realtimeEventToEventEnvelope(frame: RealtimeEventEnvelope): EventEnvelope | null {
  const base = {
    id: frame.id,
    createdAt: timestampToISO(frame.createdAt),
    actorId: frame.actorId ?? null,
    actor: null
  };

  switch (frame.event.case) {
    case 'messagePosted': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.MessagePosted,
          roomId: value.roomId,
          messageEventId: value.messageEventId,
          threadRootEventId: value.threadRootEventId ?? null
        }
      } as unknown as EventEnvelope;
    }
    case 'messageEdited': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.MessageEdited,
          roomId: value.roomId,
          messageEventId: value.messageEventId
        }
      } as unknown as EventEnvelope;
    }
    case 'messageRetracted': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.MessageRetracted,
          roomId: value.roomId,
          messageEventId: value.messageEventId,
          retractedReason: value.reason ?? ''
        }
      } as unknown as EventEnvelope;
    }
    case 'reactionAdded':
    case 'reactionRemoved': {
      const value = frame.event.value;
      const kind =
        frame.event.case === 'reactionAdded'
          ? RoomEventKind.ReactionAdded
          : RoomEventKind.ReactionRemoved;
      return {
        ...base,
        event: {
          kind,
          roomId: value.roomId,
          messageEventId: value.messageEventId,
          emoji: value.emoji
        }
      } as unknown as EventEnvelope;
    }
    case 'userTyping': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserTyping,
          roomId: value.roomId,
          typingThreadRootEventId: value.threadRootEventId ?? null
        }
      } as unknown as EventEnvelope;
    }
    case 'presenceChanged':
      return {
        ...base,
        actorId: frame.event.value.userId || base.actorId,
        event: {
          kind: RoomEventKind.PresenceChanged,
          status: presenceStatus(frame.event.value.status)
        }
      } as unknown as EventEnvelope;
    case 'roomCreated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomCreated,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'roomUpdated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomUpdated,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'roomDeleted':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomDeleted,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'roomArchived':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomArchived,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'roomUnarchived':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomUnarchived,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'userJoinedRoom':
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserJoinedRoom,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'userLeftRoom':
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserLeftRoom,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'roomUniversalChanged':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomUniversalChanged,
          roomId: frame.event.value.roomId,
          universal: frame.event.value.universal
        }
      } as unknown as EventEnvelope;
    case 'notificationCreated': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.NotificationCreated,
          notificationId: value.notificationId,
          roomId: value.roomId ?? null,
          eventId: value.eventId ?? null,
          inReplyToId: value.inReplyToId ?? null,
          silent: value.silent
        }
      } as unknown as EventEnvelope;
    }
    case 'notificationDismissed':
      return {
        ...base,
        event: {
          kind: RoomEventKind.NotificationDismissed,
          notificationId: frame.event.value.notificationId
        }
      } as unknown as EventEnvelope;
    case 'notificationLevelChanged':
      return {
        ...base,
        event: {
          kind: RoomEventKind.NotificationLevelChanged,
          nlcRoomId: frame.event.value.roomId || null,
          level: notificationLevel(frame.event.value.level),
          effectiveLevel: notificationLevel(frame.event.value.effectiveLevel)
        }
      } as unknown as EventEnvelope;
    case 'threadFollowChanged':
      return {
        ...base,
        event: {
          kind: RoomEventKind.ThreadFollowChanged,
          tfcRoomId: frame.event.value.roomId,
          tfcThreadRootEventId: frame.event.value.threadRootEventId,
          isFollowing: frame.event.value.following
        }
      } as unknown as EventEnvelope;
    case 'threadCreated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.ThreadCreated,
          roomId: frame.event.value.roomId,
          threadRootEventId: frame.event.value.threadRootEventId
        }
      } as unknown as EventEnvelope;
    case 'roomMarkedAsRead':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomMarkedAsRead,
          roomId: frame.event.value.roomId
        }
      } as unknown as EventEnvelope;
    case 'serverUpdated': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.ServerUpdated,
          name: value.name,
          description: value.description,
          logoUrl: value.logoUrl ?? null,
          bannerUrl: value.bannerUrl ?? null
        }
      } as unknown as EventEnvelope;
    }
    case 'userProfileUpdated': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserProfileUpdated,
          userId: value.userId,
          login: value.login,
          displayName: value.displayName,
          avatarUrl: value.avatarUrl ?? null
        }
      } as unknown as EventEnvelope;
    }
    case 'userCustomStatusSet': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserCustomStatusSet,
          userId: value.userId,
          setCustomStatus: {
            emoji: value.emoji,
            text: value.text,
            expiresAt: optionalTimestampToISO(value.expiresAt)
          }
        }
      } as unknown as EventEnvelope;
    }
    case 'userCustomStatusCleared':
      return {
        ...base,
        event: {
          kind: RoomEventKind.UserCustomStatusCleared,
          userId: frame.event.value.userId
        }
      } as unknown as EventEnvelope;
    case 'serverUserPreferencesUpdated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.ServerUserPreferencesUpdated,
          timezone: frame.event.value.timezone ?? null,
          timeFormat: timeFormat(frame.event.value.timeFormat)
        }
      } as unknown as EventEnvelope;
    case 'roomGroupsUpdated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.RoomGroupsUpdated,
          changed: frame.event.value.changed
        }
      } as unknown as EventEnvelope;
    case 'serverMemberDeleted':
      return {
        ...base,
        event: {
          kind: RoomEventKind.ServerMemberDeleted,
          userId: frame.event.value.userId
        }
      } as unknown as EventEnvelope;
    case 'assetProcessingStarted':
    case 'assetProcessingSucceeded':
    case 'assetProcessingFailed': {
      const value = frame.event.value;
      const kind =
        frame.event.case === 'assetProcessingStarted'
          ? RoomEventKind.AssetProcessingStarted
          : frame.event.case === 'assetProcessingSucceeded'
            ? RoomEventKind.AssetProcessingSucceeded
            : RoomEventKind.AssetProcessingFailed;
      return {
        ...base,
        event: {
          kind,
          processingRoomId: value.roomId ?? null,
          assetId: value.assetId,
          processingMessageEventId: value.messageEventId ?? null
        }
      } as unknown as EventEnvelope;
    }
    case 'assetDeleted':
      return {
        ...base,
        event: {
          kind: RoomEventKind.AssetDeleted,
          deletedRoomId: frame.event.value.roomId ?? null,
          assetId: frame.event.value.assetId
        }
      } as unknown as EventEnvelope;
    case 'callStarted':
    case 'callParticipantJoined':
    case 'callParticipantLeft':
    case 'callEnded': {
      const value = frame.event.value;
      const kind =
        frame.event.case === 'callStarted'
          ? RoomEventKind.CallStarted
          : frame.event.case === 'callParticipantJoined'
            ? RoomEventKind.CallParticipantJoined
            : frame.event.case === 'callParticipantLeft'
              ? RoomEventKind.CallParticipantLeft
              : RoomEventKind.CallEnded;
      return {
        ...base,
        event: { kind, roomId: value.roomId, callId: value.callId }
      } as unknown as EventEnvelope;
    }
    case 'mentionNotification': {
      const value = frame.event.value;
      return {
        ...base,
        actorId: value.actorUserId || base.actorId,
        event: {
          kind: RoomEventKind.MentionNotification,
          roomId: value.roomId,
          room: { name: value.roomName ?? '' },
          actor: value.actorUserId
            ? {
                id: value.actorUserId,
                displayName: value.actorDisplayName ?? ''
              }
            : null
        }
      } as unknown as EventEnvelope;
    }
    case 'newDirectMessageNotification': {
      const value = frame.event.value;
      return {
        ...base,
        actorId: value.senderId || base.actorId,
        event: {
          kind: RoomEventKind.NewDirectMessageNotification,
          roomId: value.roomId,
          sender: value.senderId
            ? {
                id: value.senderId,
                displayName: value.senderDisplayName ?? '',
                avatarUrl: value.senderAvatarUrl || null
              }
            : null,
          conversationName: value.conversationName ?? ''
        }
      } as unknown as EventEnvelope;
    }
    case 'sessionTerminated':
      return {
        ...base,
        event: {
          kind: RoomEventKind.SessionTerminated,
          reason: frame.event.value.reason
        }
      } as unknown as EventEnvelope;
    default:
      return null;
  }
}
