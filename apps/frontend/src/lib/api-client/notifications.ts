import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { authHeaders, createChattoClient } from './connect.js';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import type {
  ListNotificationOccurrencesResponse,
  NotificationMessageReference,
  NotificationOccurrence as APINotificationOccurrence
} from '@chatto/api-types/api/v1/notifications_pb';
import {
  NotificationAttentionLevel,
  NotificationDeliveryMode,
  NotificationPreferenceCategory
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

export type NotificationOccurrenceItem = {
  id: string;
  createdAt: string;
  actor: NotificationActor | null;
  /** Signal class understood by this client, or unsupported for an additive future signal. */
  signalKind: NotificationSignalKind;
  /** Whether this client understands the signal well enough to navigate it. */
  targetSupported: boolean;
  room: { id: string; name: string } | null;
  eventId: string;
  threadRootId: string | null;
  attentionLevel: NotificationAttentionLevel;
  unread: boolean;
  reactionEmoji?: string | null;
  expiresAt?: string;
};

export type NotificationGroupItem = {
  id: string;
  occurrences: NotificationOccurrenceItem[];
  openTarget: NotificationOccurrenceItem | null;
  unread: boolean;
  attentionLevel: NotificationAttentionLevel;
  latestAt: string;
  nextExpiryAt?: string | null;
};

export type NotificationOccurrencePage = {
  occurrences: NotificationOccurrenceItem[];
  /** Raw server rows consumed, which can exceed renderable rows after privacy filtering. */
  consumedCount?: number;
  unreadCount: number;
  importantUnreadCount: number;
  roomUnreadCounts: Record<string, number>;
  roomImportantUnreadCounts: Record<string, number>;
  totalCount: number;
  hasMore: boolean;
  nextExpiryAt?: string | null;
};

export { NotificationAttentionLevel, NotificationDeliveryMode, NotificationPreferenceCategory };

export const NotificationSignalKind = {
  DIRECT_MESSAGE: 'directMessageReceived',
  DIRECT_MENTION: 'directMentionReceived',
  REPLY: 'replyReceived',
  ROLE_MENTION: 'roleMentionReceived',
  HERE: 'hereMentionReceived',
  ALL: 'allMentionReceived',
  FOLLOWED_THREAD: 'followedThreadActivity',
  FOLLOWED_ROOM: 'followedRoomActivity',
  REACTION: 'reactionReceived',
  UNSUPPORTED: 'unsupported'
} as const;

export type NotificationSignalKind =
  (typeof NotificationSignalKind)[keyof typeof NotificationSignalKind];

export type NotificationPolicyItem = {
  category: NotificationPreferenceCategory;
  override: NotificationDeliveryMode | null;
  effective: NotificationDeliveryMode;
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
      if (!response.occurrence) throw new Error('Read notification was not returned');
      return notificationOccurrence(response.occurrence);
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
        category: preference.category,
        override: preference.override ?? null,
        effective: preference.effective
      }));
    },

    async setNotificationPolicyPreference(
      category: NotificationPreferenceCategory,
      override: NotificationDeliveryMode | null,
      roomId?: string
    ): Promise<NotificationPolicyItem[]> {
      const response = await client.setNotificationPolicyPreference(
        { category, override: override ?? undefined, roomId },
        { headers: headers() }
      );
      return response.preferences.map((preference) => ({
        category: preference.category,
        override: preference.override ?? null,
        effective: preference.effective
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
    consumedCount: response.occurrences.length,
    unreadCount: Number(response.unreadCount),
    importantUnreadCount: Number(response.importantUnreadCount),
    roomUnreadCounts: Object.fromEntries(
      response.roomUnreadCounts.map((count) => [count.roomId, Number(count.unreadCount)])
    ),
    roomImportantUnreadCounts: Object.fromEntries(
      response.roomUnreadCounts.map((count) => [count.roomId, Number(count.importantUnreadCount)])
    ),
    totalCount: Number(response.page?.totalCount ?? 0),
    hasMore: response.page?.hasMore ?? false,
    nextExpiryAt: response.nextExpiryAt?.toDate().toISOString() ?? null
  };
}

export function notificationOccurrence(
  item: APINotificationOccurrence
): NotificationOccurrenceItem {
  const mapped = notificationSignal(item);
  const target = mapped.message;
  const actor = notificationActor(item.actor);
  return {
    id: item.id,
    createdAt: item.createdAt?.toDate().toISOString() ?? new Date(0).toISOString(),
    actor,
    signalKind: mapped.kind,
    targetSupported: mapped.supported,
    room: target?.room ? { id: target.room.id, name: target.room.name } : null,
    eventId: target?.eventId ?? '',
    threadRootId: target?.threadRootEventId ?? null,
    attentionLevel: notificationAttentionLevel(item.attentionLevel),
    unread: item.unread,
    reactionEmoji: mapped.reactionEmoji,
    expiresAt: item.expiresAt?.toDate().toISOString() ?? new Date(0).toISOString()
  };
}

function notificationAttentionLevel(level: NotificationAttentionLevel): NotificationAttentionLevel {
  // Ambient is the only low-attention value. Unknown future levels must retain
  // the stronger badge/highlight behavior in an older client.
  return level === NotificationAttentionLevel.AMBIENT
    ? NotificationAttentionLevel.AMBIENT
    : NotificationAttentionLevel.IMPORTANT;
}

function notificationSignal(item: APINotificationOccurrence): {
  supported: boolean;
  kind: NotificationSignalKind;
  message: NotificationMessageReference | null;
  reactionEmoji: string | null;
} {
  const kind = item.signal?.kind;
  switch (kind?.case) {
    case 'directMessageReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.DIRECT_MESSAGE,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'directMentionReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.DIRECT_MENTION,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'replyReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.REPLY,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'roleMentionReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.ROLE_MENTION,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'hereMentionReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.HERE,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'allMentionReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.ALL,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'followedThreadActivity':
      return {
        supported: true,
        kind: NotificationSignalKind.FOLLOWED_THREAD,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'followedRoomActivity':
      return {
        supported: true,
        kind: NotificationSignalKind.FOLLOWED_ROOM,
        message: kind.value.message ?? null,
        reactionEmoji: null
      };
    case 'reactionReceived':
      return {
        supported: true,
        kind: NotificationSignalKind.REACTION,
        message: kind.value.message ?? null,
        reactionEmoji: kind.value.emoji || null
      };
    default:
      return {
        supported: false,
        kind: NotificationSignalKind.UNSUPPORTED,
        message: null,
        reactionEmoji: null
      };
  }
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
        unread: occurrences.some((occurrence) => occurrence.unread),
        attentionLevel,
        latestAt: occurrences[0]?.createdAt ?? new Date(0).toISOString(),
        nextExpiryAt: expiries.sort()[0] ?? null
      };
    })
    .sort((a, b) => b.latestAt.localeCompare(a.latestAt));
}

function notificationPresentationGroupKey(occurrence: NotificationOccurrenceItem): string {
  if (occurrence.targetSupported === false) return `occurrence:${occurrence.id}`;
  const roomId = occurrence.room?.id ?? '';
  if (occurrence.signalKind === NotificationSignalKind.DIRECT_MESSAGE) {
    return `dm:${roomId}`;
  }
  if (
    occurrence.signalKind === NotificationSignalKind.REPLY ||
    occurrence.signalKind === NotificationSignalKind.DIRECT_MENTION ||
    occurrence.signalKind === NotificationSignalKind.ROLE_MENTION ||
    occurrence.signalKind === NotificationSignalKind.HERE ||
    occurrence.signalKind === NotificationSignalKind.ALL
  ) {
    return `occurrence:${occurrence.id}`;
  }
  if (occurrence.signalKind === NotificationSignalKind.REACTION) {
    return `reaction:${roomId}:${occurrence.threadRootId ?? ''}:${occurrence.eventId}`;
  }
  if (occurrence.signalKind === NotificationSignalKind.FOLLOWED_THREAD) {
    return `thread:${roomId}:${occurrence.threadRootId ?? occurrence.eventId}`;
  }
  if (occurrence.signalKind === NotificationSignalKind.FOLLOWED_ROOM) {
    return `room:${roomId}`;
  }
  // Unknown future causes stay exact until the client deliberately chooses a
  // safe presentation boundary for them.
  return `occurrence:${occurrence.id}`;
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
