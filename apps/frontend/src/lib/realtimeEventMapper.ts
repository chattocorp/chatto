import {
  NotificationLevel as GqlNotificationLevel,
  PresenceStatus as GqlPresenceStatus,
  TimeFormat
} from '$lib/gql/graphql';
import { NotificationLevel as ApiNotificationLevel } from '$lib/pb/chatto/api/v1/notification_preferences_pb';
import {
  RealtimeEventEnvelope,
  RealtimeHeartbeat,
  RealtimePresenceStatus,
  RealtimeTimeFormat
} from '$lib/pb/chatto/api/v1/realtime_pb';
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

function presenceStatus(status: RealtimePresenceStatus): GqlPresenceStatus {
  switch (status) {
    case RealtimePresenceStatus.AWAY:
      return GqlPresenceStatus.Away;
    case RealtimePresenceStatus.DO_NOT_DISTURB:
      return GqlPresenceStatus.DoNotDisturb;
    case RealtimePresenceStatus.ONLINE:
      return GqlPresenceStatus.Online;
    case RealtimePresenceStatus.OFFLINE:
    case RealtimePresenceStatus.UNSPECIFIED:
    default:
      return GqlPresenceStatus.Offline;
  }
}

function timeFormat(format: RealtimeTimeFormat): TimeFormat {
  switch (format) {
    case RealtimeTimeFormat.REALTIME_TIME_FORMAT_12H:
      return TimeFormat.TwelveHour;
    case RealtimeTimeFormat.REALTIME_TIME_FORMAT_24H:
      return TimeFormat.TwentyFourHour;
    case RealtimeTimeFormat.REALTIME_TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}

export function realtimeHeartbeatToEventEnvelope(frame: RealtimeHeartbeat): EventEnvelope {
  return {
    __typename: 'Event',
    id: frame.id,
    createdAt: timestampToISO(frame.createdAt),
    actorId: null,
    actor: null,
    event: { __typename: 'HeartbeatEvent', alive: true }
  } as unknown as EventEnvelope;
}

export function realtimeEventToEventEnvelope(frame: RealtimeEventEnvelope): EventEnvelope | null {
  const base = {
    __typename: 'Event',
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
          __typename: 'MessagePostedEvent',
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
          __typename: 'MessageEditedEvent',
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
          __typename: 'MessageRetractedEvent',
          roomId: value.roomId,
          messageEventId: value.messageEventId,
          retractedReason: value.reason ?? ''
        }
      } as unknown as EventEnvelope;
    }
    case 'reactionAdded':
    case 'reactionRemoved': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          __typename:
            frame.event.case === 'reactionAdded' ? 'ReactionAddedEvent' : 'ReactionRemovedEvent',
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
          __typename: 'UserTypingEvent',
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
          __typename: 'PresenceChangedEvent',
          status: presenceStatus(frame.event.value.status)
        }
      } as unknown as EventEnvelope;
    case 'roomCreated':
      return {
        ...base,
        event: { __typename: 'RoomCreatedEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'roomUpdated':
      return {
        ...base,
        event: { __typename: 'RoomUpdatedEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'roomDeleted':
      return {
        ...base,
        event: { __typename: 'RoomDeletedEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'roomArchived':
      return {
        ...base,
        event: { __typename: 'RoomArchivedEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'roomUnarchived':
      return {
        ...base,
        event: { __typename: 'RoomUnarchivedEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'userJoinedRoom':
      return {
        ...base,
        event: { __typename: 'UserJoinedRoomEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'userLeftRoom':
      return {
        ...base,
        event: { __typename: 'UserLeftRoomEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'roomUniversalChanged':
      return {
        ...base,
        event: {
          __typename: 'RoomUniversalChangedEvent',
          roomId: frame.event.value.roomId,
          universal: frame.event.value.universal
        }
      } as unknown as EventEnvelope;
    case 'notificationCreated': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          __typename: 'NotificationCreatedEvent',
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
          __typename: 'NotificationDismissedEvent',
          notificationId: frame.event.value.notificationId
        }
      } as unknown as EventEnvelope;
    case 'notificationLevelChanged':
      return {
        ...base,
        event: {
          __typename: 'NotificationLevelChangedEvent',
          nlcRoomId: frame.event.value.roomId || null,
          level: notificationLevel(frame.event.value.level),
          effectiveLevel: notificationLevel(frame.event.value.effectiveLevel)
        }
      } as unknown as EventEnvelope;
    case 'threadFollowChanged':
      return {
        ...base,
        event: {
          __typename: 'ThreadFollowChangedEvent',
          tfcRoomId: frame.event.value.roomId,
          tfcThreadRootEventId: frame.event.value.threadRootEventId,
          isFollowing: frame.event.value.following
        }
      } as unknown as EventEnvelope;
    case 'threadCreated':
      return {
        ...base,
        event: {
          __typename: 'ThreadCreatedEvent',
          roomId: frame.event.value.roomId,
          threadRootEventId: frame.event.value.threadRootEventId
        }
      } as unknown as EventEnvelope;
    case 'roomMarkedAsRead':
      return {
        ...base,
        event: { __typename: 'RoomMarkedAsReadEvent', roomId: frame.event.value.roomId }
      } as unknown as EventEnvelope;
    case 'serverUpdated': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          __typename: 'ServerUpdatedEvent',
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
          __typename: 'UserProfileUpdatedEvent',
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
          __typename: 'UserCustomStatusSetEvent',
          userId: value.userId,
          setCustomStatus: {
            __typename: 'CustomUserStatus',
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
          __typename: 'UserCustomStatusClearedEvent',
          userId: frame.event.value.userId
        }
      } as unknown as EventEnvelope;
    case 'serverUserPreferencesUpdated':
      return {
        ...base,
        event: {
          __typename: 'ServerUserPreferencesUpdatedEvent',
          timezone: frame.event.value.timezone ?? null,
          timeFormat: timeFormat(frame.event.value.timeFormat)
        }
      } as unknown as EventEnvelope;
    case 'roomGroupsUpdated':
      return {
        ...base,
        event: { __typename: 'RoomGroupsUpdatedEvent', changed: frame.event.value.changed }
      } as unknown as EventEnvelope;
    case 'serverMemberDeleted':
      return {
        ...base,
        event: { __typename: 'ServerMemberDeletedEvent', userId: frame.event.value.userId }
      } as unknown as EventEnvelope;
    case 'assetProcessingStarted':
    case 'assetProcessingSucceeded':
    case 'assetProcessingFailed': {
      const value = frame.event.value;
      const typename =
        frame.event.case === 'assetProcessingStarted'
          ? 'AssetProcessingStartedEvent'
          : frame.event.case === 'assetProcessingSucceeded'
            ? 'AssetProcessingSucceededEvent'
            : 'AssetProcessingFailedEvent';
      return {
        ...base,
        event: {
          __typename: typename,
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
          __typename: 'AssetDeletedEvent',
          deletedRoomId: frame.event.value.roomId ?? null,
          assetId: frame.event.value.assetId
        }
      } as unknown as EventEnvelope;
    case 'callStarted':
    case 'callParticipantJoined':
    case 'callParticipantLeft':
    case 'callEnded': {
      const value = frame.event.value;
      const typename =
        frame.event.case === 'callStarted'
          ? 'CallStartedEvent'
          : frame.event.case === 'callParticipantJoined'
            ? 'CallParticipantJoinedEvent'
            : frame.event.case === 'callParticipantLeft'
              ? 'CallParticipantLeftEvent'
              : 'CallEndedEvent';
      return {
        ...base,
        event: { __typename: typename, roomId: value.roomId, callId: value.callId }
      } as unknown as EventEnvelope;
    }
    case 'mentionNotification': {
      const value = frame.event.value;
      return {
        ...base,
        actorId: value.actorUserId || base.actorId,
        event: {
          __typename: 'MentionNotificationEvent',
          roomId: value.roomId,
          room: { __typename: 'Room', name: value.roomName ?? '' },
          actor: value.actorUserId
            ? { __typename: 'User', id: value.actorUserId, displayName: value.actorDisplayName ?? '' }
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
          __typename: 'NewDirectMessageNotificationEvent',
          roomId: value.roomId,
          sender: value.senderId
            ? {
                __typename: 'User',
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
        event: { __typename: 'SessionTerminatedEvent', reason: frame.event.value.reason }
      } as unknown as EventEnvelope;
    default:
      return null;
  }
}
