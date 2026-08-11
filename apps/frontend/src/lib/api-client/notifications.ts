import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { authHeaders, createChattoClient } from './connect.js';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import type {
  ListNotificationGroupsResponse,
  NotificationGroup as APINotificationGroup,
  NotificationOccurrence as APINotificationOccurrence
} from '@chatto/api-types/api/v1/notifications_pb';
import {
  NotificationDeliveryIntensity,
  NotificationInboxState,
  NotificationReason,
  NotificationView
} from '@chatto/api-types/api/v1/notifications_pb';
import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { presenceStatusOrOffline } from './enumDefaults.js';
export type NotificationAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type NotificationActor = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
  customStatus?: {
    emoji: string;
    text: string;
    expiresAt?: string | null;
  } | null;
};

export const NotificationItemKind = {
  DirectMessage: 'directMessage',
  Mention: 'mention',
  Reply: 'reply',
  RoomMessage: 'roomMessage'
} as const;

export type NotificationItemKind = (typeof NotificationItemKind)[keyof typeof NotificationItemKind];

export type DirectMessageNotificationItem = {
  kind: typeof NotificationItemKind.DirectMessage;
  id: string;
  createdAt: string;
  actor?: NotificationActor | null;
  summary: string;
  room: { id: string };
  eventId?: string | null;
};

export type MentionNotificationItem = {
  kind: typeof NotificationItemKind.Mention;
  id: string;
  createdAt: string;
  actor?: NotificationActor | null;
  summary: string;
  mentionRoom: { id: string; name: string } | null;
  mentionEventId: string;
  mentionInThread?: string | null;
};

export type ReplyNotificationItem = {
  kind: typeof NotificationItemKind.Reply;
  id: string;
  createdAt: string;
  actor?: NotificationActor | null;
  summary: string;
  replyRoom: { id: string; name: string } | null;
  replyEventId: string;
  inReplyToId: string;
  replyInThread?: string | null;
};

export type RoomMessageNotificationItem = {
  kind: typeof NotificationItemKind.RoomMessage;
  id: string;
  createdAt: string;
  actor?: NotificationActor | null;
  summary: string;
  roomMsgRoom: { id: string; name: string } | null;
  roomMsgEventId: string;
  roomMsgThreadRootId?: string | null;
};

export type NotificationItem =
  | DirectMessageNotificationItem
  | MentionNotificationItem
  | ReplyNotificationItem
  | RoomMessageNotificationItem;

export type NotificationOccurrenceItem = {
  id: string;
  sourceEventId: string;
  createdAt: string;
  actor: NotificationActor | null;
  room: { id: string; name: string } | null;
  eventId: string;
  threadRootId: string | null;
  parentEventId: string | null;
  reasons: NotificationReason[];
  reasonMatches: Array<{
    reason: NotificationReason;
    intensity: NotificationDeliveryIntensity;
  }>;
  inboxState: NotificationInboxState;
  expiresAt?: string;
};

export type NotificationGroupItem = {
  id: string;
  occurrences: NotificationOccurrenceItem[];
  openTarget: NotificationOccurrenceItem | null;
  unread: boolean;
  occurrenceCount: number;
  latestAt: string;
  reasons: NotificationReason[];
  nextExpiryAt?: string | null;
};

export type NotificationGroupPage = {
  groups: NotificationGroupItem[];
  unreadGroupCount: number;
  roomUnreadGroupCounts: Record<string, number>;
  totalCount: number;
  hasMore: boolean;
  nextInboxExpiryAt?: string | null;
};

export {
  NotificationDeliveryIntensity,
  NotificationInboxState,
  NotificationReason,
  NotificationView
};
export type NotificationPolicyItem = {
  reason: NotificationReason;
  serverIntensity: NotificationDeliveryIntensity;
  roomIntensity: NotificationDeliveryIntensity;
  effectiveIntensity: NotificationDeliveryIntensity;
};

export function createNotificationAPI(config: NotificationAPIConfig) {
  const client = createChattoClient(NotificationService, config);
  const headers = () => authHeaders(config);

  return {
    async listNotificationGroups(
      view = NotificationView.INBOX,
      limit = 50,
      offset = 0
    ): Promise<NotificationGroupPage> {
      return mapNotificationGroupPage(
        await client.listNotificationGroups(
          { view, page: { limit, offset } },
          { headers: headers() }
        )
      );
    },

    async updateNotificationGroup(
      groupId: string,
      view: NotificationView,
      update: { inboxState?: NotificationInboxState }
    ): Promise<void> {
      await client.updateNotificationGroup({ groupId, view, ...update }, { headers: headers() });
    },

    async updateNotificationOccurrence(
      notificationId: string,
      update: { inboxState?: NotificationInboxState }
    ): Promise<NotificationOccurrenceItem> {
      const response = await client.updateNotificationOccurrence(
        { notificationId, ...update },
        { headers: headers() }
      );
      if (!response.notification) throw new Error('Updated notification was not returned');
      return notificationOccurrence(response.notification);
    },

    async deleteNotificationOccurrence(notificationId: string): Promise<boolean> {
      const response = await client.deleteNotificationOccurrence(
        { notificationId },
        { headers: headers() }
      );
      return response.deleted;
    },

    async deleteNotificationGroup(groupId: string, view: NotificationView): Promise<number> {
      return Number(
        (await client.deleteNotificationGroup({ groupId, view }, { headers: headers() }))
          .deletedCount
      );
    },

    async getNotificationPolicy(roomId?: string): Promise<NotificationPolicyItem[]> {
      const response = await client.getNotificationPolicy({ roomId }, { headers: headers() });
      return response.preferences.map((preference) => ({
        reason: preference.reason,
        serverIntensity: preference.serverIntensity,
        roomIntensity: preference.roomIntensity,
        effectiveIntensity: preference.effectiveIntensity
      }));
    },

    async setNotificationPolicyPreference(
      reason: NotificationReason,
      intensity: NotificationDeliveryIntensity,
      roomId?: string
    ): Promise<NotificationPolicyItem[]> {
      const response = await client.setNotificationPolicyPreference(
        { reason, intensity, roomId },
        { headers: headers() }
      );
      return response.preferences.map((preference) => ({
        reason: preference.reason,
        serverIntensity: preference.serverIntensity,
        roomIntensity: preference.roomIntensity,
        effectiveIntensity: preference.effectiveIntensity
      }));
    }
  };
}

export type NotificationAPI = ReturnType<typeof createNotificationAPI>;

export function mapNotificationGroupPage(
  response: ListNotificationGroupsResponse
): NotificationGroupPage {
  return {
    groups: response.groups.map(notificationGroup),
    unreadGroupCount: Number(response.unreadGroupCount),
    roomUnreadGroupCounts: Object.fromEntries(
      response.roomUnreadGroupCounts.map((count) => [
        count.roomId,
        Number(count.unreadGroupCount)
      ])
    ),
    totalCount: Number(response.page?.totalCount ?? 0),
    hasMore: response.page?.hasMore ?? false,
    nextInboxExpiryAt: response.nextInboxExpiryAt?.toDate().toISOString() ?? null
  };
}

function notificationGroup(group: APINotificationGroup): NotificationGroupItem {
  const occurrences = group.occurrences.map(notificationOccurrence);
  const targetEventId = group.openTarget?.eventId;
  return {
    id: group.id,
    occurrences,
    openTarget:
      occurrences.find((occurrence) => occurrence.id === group.openNotificationId) ??
      occurrences.find((occurrence) => occurrence.eventId === targetEventId) ??
      occurrences[0] ??
      null,
    unread: group.unread,
    occurrenceCount: Number(group.occurrenceCount),
    latestAt: group.latestAt?.toDate().toISOString() ?? new Date(0).toISOString(),
    reasons: [...group.reasons],
    nextExpiryAt: group.nextExpiryAt?.toDate().toISOString() ?? null
  };
}

export function notificationOccurrence(
  item: APINotificationOccurrence
): NotificationOccurrenceItem {
  const actor = notificationActor(item.actor);
  const reasonMatches = item.reasons.map((match) => ({
    reason: match.reason,
    intensity: match.intensity
  }));
  const reasons = reasonMatches.map((match) => match.reason);
  return {
    id: item.id,
    sourceEventId: item.sourceEventId,
    createdAt: item.createdAt?.toDate().toISOString() ?? new Date(0).toISOString(),
    actor,
    room: item.target?.room ? { id: item.target.room.id, name: item.target.room.name } : null,
    eventId: item.target?.eventId ?? '',
    threadRootId: item.target?.threadRootEventId ?? null,
    parentEventId: item.target?.parentEventId ?? null,
    reasons,
    reasonMatches,
    inboxState: item.inboxState,
    expiresAt: item.expiresAt?.toDate().toISOString() ?? new Date(0).toISOString()
  };
}

export function occurrenceAsNotificationItem(item: NotificationOccurrenceItem): NotificationItem {
  const base = {
    id: item.id,
    createdAt: item.createdAt,
    actor: item.actor,
    // Compact sidebar presentation consumers require this field, while the
    // notification centre renders structured reasons through the active locale.
    summary: ''
  };
  if (item.reasons.includes(NotificationReason.DIRECT_MESSAGE)) {
    return {
      kind: NotificationItemKind.DirectMessage,
      ...base,
      room: { id: item.room?.id ?? '' },
      eventId: item.eventId
    };
  }
  if (item.reasons.includes(NotificationReason.REPLY)) {
    return {
      kind: NotificationItemKind.Reply,
      ...base,
      replyRoom: item.room,
      replyEventId: item.eventId,
      inReplyToId: item.parentEventId ?? '',
      replyInThread: item.threadRootId
    };
  }
  if (
    item.reasons.includes(NotificationReason.DIRECT_MENTION) ||
    item.reasons.includes(NotificationReason.ROLE_MENTION) ||
    item.reasons.includes(NotificationReason.HERE) ||
    item.reasons.includes(NotificationReason.ALL)
  ) {
    return {
      kind: NotificationItemKind.Mention,
      ...base,
      mentionRoom: item.room,
      mentionEventId: item.eventId,
      mentionInThread: item.threadRootId
    };
  }
  if (item.reasons.includes(NotificationReason.FOLLOWED_THREAD)) {
    return {
      kind: NotificationItemKind.Reply,
      ...base,
      replyRoom: item.room,
      replyEventId: item.eventId,
      inReplyToId: item.parentEventId ?? '',
      replyInThread: item.threadRootId
    };
  }
  return {
    kind: NotificationItemKind.RoomMessage,
    ...base,
    roomMsgRoom: item.room,
    roomMsgEventId: item.eventId,
    roomMsgThreadRootId: item.threadRootId
  };
}

function notificationActor(actor: APIUser | undefined): NotificationActor | null {
  if (!actor) return null;
  return {
    id: actor.id,
    login: actor.login,
    displayName: actor.displayName,
    deleted: actor.deleted,
    avatarUrl: actor.avatarUrl ?? null,
    presenceStatus: presenceStatusOrOffline(actor.presenceStatus),
    customStatus: actor.customStatus
      ? {
          emoji: actor.customStatus.emoji,
          text: actor.customStatus.text,
          expiresAt: actor.customStatus.expiresAt?.toDate().toISOString() ?? null
        }
      : null
  };
}
