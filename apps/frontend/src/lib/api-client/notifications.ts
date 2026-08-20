import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { authHeaders, createChattoClient } from './connect.js';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import type {
  ListNotificationOccurrencesResponse,
  NotificationDeliveryModes as APINotificationDeliveryModes,
  NotificationMessageReference,
  NotificationOccurrence as APINotificationOccurrence,
  NotificationPolicy as APINotificationPolicy
} from '@chatto/api-types/api/v1/notifications_pb';
import {
  NotificationAttentionLevel,
  NotificationDeliveryMode
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

export { NotificationAttentionLevel, NotificationDeliveryMode };

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

type NotificationPolicyShape<Value> = {
  directMessages: Value;
  directMentions: Value;
  replies: Value;
  roleMentions: Value;
  hereMentions: Value;
  allMentions: Value;
  followedThreads: Value;
  followedRooms: Value;
  reactions: Value;
};

export type NotificationPolicyModes = NotificationPolicyShape<NotificationDeliveryMode>;
export type NotificationPolicyOverrides = NotificationPolicyShape<NotificationDeliveryMode | null>;
export type NotificationPolicyField = keyof NotificationPolicyOverrides;
export type NotificationPolicyPatch = Partial<NotificationPolicyOverrides>;

export type NotificationPolicy = {
  overrides: NotificationPolicyOverrides;
  effective: NotificationPolicyModes;
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

    async getNotificationPolicy(roomId?: string): Promise<NotificationPolicy> {
      const response = await client.getNotificationPolicy({ roomId }, { headers: headers() });
      return notificationPolicy(response.policy);
    },

    async updateNotificationPolicy(
      patch: NotificationPolicyPatch,
      roomId?: string
    ): Promise<NotificationPolicy> {
      const { overrides, paths } = notificationPolicyUpdate(patch);
      if (paths.length === 0) throw new Error('Notification policy update is empty');
      const response = await client.updateNotificationPolicy(
        { roomId, overrides, updateMask: { paths } },
        { headers: headers() }
      );
      return notificationPolicy(response.policy);
    }
  };
}

export type NotificationAPI = ReturnType<typeof createNotificationAPI>;

function notificationPolicy(policy: APINotificationPolicy | undefined): NotificationPolicy {
  return {
    overrides: {
      directMessages: policy?.overrides?.directMessages ?? null,
      directMentions: policy?.overrides?.directMentions ?? null,
      replies: policy?.overrides?.replies ?? null,
      roleMentions: policy?.overrides?.roleMentions ?? null,
      hereMentions: policy?.overrides?.hereMentions ?? null,
      allMentions: policy?.overrides?.allMentions ?? null,
      followedThreads: policy?.overrides?.followedThreads ?? null,
      followedRooms: policy?.overrides?.followedRooms ?? null,
      reactions: policy?.overrides?.reactions ?? null
    },
    effective: {
      directMessages: requiredNotificationDeliveryMode(
        policy?.effective?.directMessages,
        'direct_messages'
      ),
      directMentions: requiredNotificationDeliveryMode(
        policy?.effective?.directMentions,
        'direct_mentions'
      ),
      replies: requiredNotificationDeliveryMode(policy?.effective?.replies, 'replies'),
      roleMentions: requiredNotificationDeliveryMode(
        policy?.effective?.roleMentions,
        'role_mentions'
      ),
      hereMentions: requiredNotificationDeliveryMode(
        policy?.effective?.hereMentions,
        'here_mentions'
      ),
      allMentions: requiredNotificationDeliveryMode(policy?.effective?.allMentions, 'all_mentions'),
      followedThreads: requiredNotificationDeliveryMode(
        policy?.effective?.followedThreads,
        'followed_threads'
      ),
      followedRooms: requiredNotificationDeliveryMode(
        policy?.effective?.followedRooms,
        'followed_rooms'
      ),
      reactions: requiredNotificationDeliveryMode(policy?.effective?.reactions, 'reactions')
    }
  };
}

function requiredNotificationDeliveryMode(
  mode: NotificationDeliveryMode | undefined,
  field: string
): NotificationDeliveryMode {
  if (
    mode !== NotificationDeliveryMode.OFF &&
    mode !== NotificationDeliveryMode.SILENT &&
    mode !== NotificationDeliveryMode.ALERT
  ) {
    throw new Error(`Notification policy is missing an effective ${field} mode`);
  }
  return mode;
}

function notificationPolicyUpdate(patch: NotificationPolicyPatch): {
  overrides: Partial<APINotificationDeliveryModes>;
  paths: string[];
} {
  const overrides: Partial<APINotificationDeliveryModes> = {};
  const paths: string[] = [];

  addNotificationPolicyUpdate(patch, overrides, paths, 'directMessages', 'direct_messages');
  addNotificationPolicyUpdate(patch, overrides, paths, 'directMentions', 'direct_mentions');
  addNotificationPolicyUpdate(patch, overrides, paths, 'replies', 'replies');
  addNotificationPolicyUpdate(patch, overrides, paths, 'roleMentions', 'role_mentions');
  addNotificationPolicyUpdate(patch, overrides, paths, 'hereMentions', 'here_mentions');
  addNotificationPolicyUpdate(patch, overrides, paths, 'allMentions', 'all_mentions');
  addNotificationPolicyUpdate(patch, overrides, paths, 'followedThreads', 'followed_threads');
  addNotificationPolicyUpdate(patch, overrides, paths, 'followedRooms', 'followed_rooms');
  addNotificationPolicyUpdate(patch, overrides, paths, 'reactions', 'reactions');

  return { overrides, paths };
}

function addNotificationPolicyUpdate(
  patch: NotificationPolicyPatch,
  overrides: Partial<APINotificationDeliveryModes>,
  paths: string[],
  field: NotificationPolicyField,
  path: string
) {
  if (!Object.hasOwn(patch, field)) return;
  paths.push(path);
  const mode = patch[field];
  if (mode !== null && mode !== undefined) overrides[field] = mode;
}

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
