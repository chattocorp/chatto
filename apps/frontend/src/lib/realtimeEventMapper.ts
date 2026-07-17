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
