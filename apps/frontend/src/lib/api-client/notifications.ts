import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { authHeaders, createChattoClient } from './connect.js';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import type {
  ListNotificationOccurrencesResponse,
  NotificationOccurrence as APINotificationOccurrence
} from '@chatto/api-types/api/v1/notifications_pb';
import {
  NotificationAttentionLevel,
  NotificationDeliveryIntensity,
  NotificationReason
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
  attentionLevel: NotificationAttentionLevel;
  unread: boolean;
  reactionEmoji?: string | null;
  threadRootMessageExcerpt?: string | null;
  expiresAt?: string;
};

export type NotificationGroupItem = {
  id: string;
  occurrences: NotificationOccurrenceItem[];
  openTarget: NotificationOccurrenceItem | null;
  threadRootMessageExcerpt?: string | null;
  unread: boolean;
  attentionLevel: NotificationAttentionLevel;
  occurrenceCount: number;
  latestAt: string;
  reasons: NotificationReason[];
  nextExpiryAt?: string | null;
};

export type NotificationOccurrencePage = {
  occurrences: NotificationOccurrenceItem[];
  unreadCount: number;
  importantUnreadCount: number;
  roomUnreadCounts: Record<string, number>;
  roomImportantUnreadCounts: Record<string, number>;
  totalCount: number;
  hasMore: boolean;
  nextExpiryAt?: string | null;
};

export { NotificationAttentionLevel, NotificationDeliveryIntensity, NotificationReason };
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
    async listNotificationOccurrences(limit = 50, offset = 0): Promise<NotificationOccurrencePage> {
      return mapNotificationOccurrencePage(
        await client.listNotificationOccurrences(
          { page: { limit, offset } },
          { headers: headers() }
        )
      );
    },

    async markNotificationRead(notificationId: string): Promise<NotificationOccurrenceItem> {
      const response = await client.markNotificationRead(
        { notificationId },
        { headers: headers() }
      );
      if (!response.notification) throw new Error('Read notification was not returned');
      return notificationOccurrence(response.notification);
    },

    async deleteNotificationOccurrence(notificationId: string): Promise<boolean> {
      const response = await client.deleteNotificationOccurrence(
        { notificationId },
        { headers: headers() }
      );
      return response.deleted;
    },

    async batchDeleteNotificationOccurrences(notificationIds: string[]): Promise<number> {
      const uniqueIds = [...new Set(notificationIds)];
      let deletedCount = 0;
      for (let offset = 0; offset < uniqueIds.length; offset += 100) {
        const response = await client.batchDeleteNotificationOccurrences(
          { notificationIds: uniqueIds.slice(offset, offset + 100) },
          { headers: headers() }
        );
        deletedCount += Number(response.deletedCount);
      }
      return deletedCount;
    },

    async deleteAllNotificationOccurrences(): Promise<number> {
      return Number(
        (await client.deleteAllNotificationOccurrences({}, { headers: headers() })).deletedCount
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

export function mapNotificationOccurrencePage(
  response: ListNotificationOccurrencesResponse
): NotificationOccurrencePage {
  return {
    occurrences: response.occurrences.map(notificationOccurrence),
    unreadCount: Number(response.unreadCount),
    // An absent optional field identifies an older Notifications 2.0 server.
    // Preserve its all-orange behavior instead of understating importance.
    importantUnreadCount: Number(response.importantUnreadCount ?? response.unreadCount),
    roomUnreadCounts: Object.fromEntries(
      response.roomUnreadCounts.map((count) => [count.roomId, Number(count.unreadCount)])
    ),
    roomImportantUnreadCounts: Object.fromEntries(
      response.roomUnreadCounts.map((count) => [
        count.roomId,
        Number(count.importantUnreadCount ?? count.unreadCount)
      ])
    ),
    totalCount: Number(response.page?.totalCount ?? 0),
    hasMore: response.page?.hasMore ?? false,
    nextExpiryAt: response.nextExpiryAt?.toDate().toISOString() ?? null
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
    attentionLevel: effectiveNotificationAttentionLevel(item.attentionLevel, reasons),
    unread: item.unread,
    reactionEmoji: item.reactionEmoji || null,
    threadRootMessageExcerpt: item.threadRootMessageExcerpt ?? null,
    expiresAt: item.expiresAt?.toDate().toISOString() ?? new Date(0).toISOString()
  };
}

/** Derive temporary presentation groups from exact server occurrences. */
export function groupNotificationOccurrences(
  source: NotificationOccurrenceItem[]
): NotificationGroupItem[] {
  const groups = new Map<string, NotificationOccurrenceItem[]>();
  for (const occurrence of source) {
    const key = notificationPresentationGroupKey(occurrence);
    const current = groups.get(key);
    if (current) current.push(occurrence);
    else groups.set(key, [occurrence]);
  }
  return [...groups.entries()]
    .map(([id, unsorted]) => {
      const occurrences = [...unsorted].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
      const openTarget =
        occurrences.find((occurrence) => occurrence.unread) ?? occurrences[0] ?? null;
      const reasons = [...new Set(occurrences.flatMap((occurrence) => occurrence.reasons))].sort();
      const attentionLevel = occurrences
        .filter((occurrence) => occurrence.unread)
        .reduce(
          (strongest, occurrence) => Math.max(strongest, occurrence.attentionLevel),
          NotificationAttentionLevel.UNSPECIFIED
        ) as NotificationAttentionLevel;
      const expiries = occurrences.flatMap((occurrence) =>
        occurrence.expiresAt ? [occurrence.expiresAt] : []
      );
      return {
        id,
        occurrences,
        openTarget,
        threadRootMessageExcerpt: openTarget?.threadRootMessageExcerpt ?? null,
        unread: occurrences.some((occurrence) => occurrence.unread),
        attentionLevel,
        occurrenceCount: occurrences.length,
        latestAt: occurrences[0]?.createdAt ?? new Date(0).toISOString(),
        reasons,
        nextExpiryAt: expiries.sort()[0] ?? null
      };
    })
    .sort((a, b) => b.latestAt.localeCompare(a.latestAt));
}

/** Resolve additive attention metadata, conservatively supporting older servers. */
export function effectiveNotificationAttentionLevel(
  stored: NotificationAttentionLevel,
  reasons: NotificationReason[]
): NotificationAttentionLevel {
  if (
    stored === NotificationAttentionLevel.AMBIENT ||
    stored === NotificationAttentionLevel.IMPORTANT
  ) {
    return stored;
  }
  return reasons.length > 0 && reasons.every((reason) => reason === NotificationReason.REACTION)
    ? NotificationAttentionLevel.AMBIENT
    : NotificationAttentionLevel.IMPORTANT;
}

function notificationPresentationGroupKey(occurrence: NotificationOccurrenceItem): string {
  const roomId = occurrence.room?.id ?? '';
  if (occurrence.reasons.includes(NotificationReason.DIRECT_MESSAGE)) {
    return `dm:${roomId}`;
  }
  if (
    occurrence.reasons.includes(NotificationReason.REPLY) ||
    occurrence.reasons.includes(NotificationReason.DIRECT_MENTION) ||
    occurrence.reasons.includes(NotificationReason.ROLE_MENTION) ||
    occurrence.reasons.includes(NotificationReason.HERE) ||
    occurrence.reasons.includes(NotificationReason.ALL)
  ) {
    return `occurrence:${occurrence.id}`;
  }
  if (occurrence.reasons.includes(NotificationReason.REACTION)) {
    return `reaction:${roomId}:${occurrence.threadRootId ?? ''}:${occurrence.eventId}`;
  }
  if (occurrence.reasons.includes(NotificationReason.FOLLOWED_THREAD)) {
    return `thread:${roomId}:${occurrence.threadRootId ?? occurrence.eventId}`;
  }
  if (occurrence.reasons.includes(NotificationReason.FOLLOWED_ROOM)) {
    return `room:${roomId}`;
  }
  // Unknown future causes stay exact until the client deliberately chooses a
  // safe presentation boundary for them.
  return `occurrence:${occurrence.id}`;
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
