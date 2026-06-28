import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { NotificationService } from '$lib/pb/chatto/api/v1/notifications_connect';
import type {
  ListRoomNotificationsResponse,
  ListNotificationsResponse,
  NotificationItem as APINotificationItem
} from '$lib/pb/chatto/api/v1/notifications_pb';
import type { UserPresenceSummary as APIUserPresenceSummary } from '$lib/pb/chatto/api/v1/users_pb';
import { PresenceStatus as APIPresenceStatus } from '$lib/pb/chatto/api/v1/presence_pb';
import { PresenceStatus } from '$lib/render/types';

export type NotificationAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
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
};

export type NotificationItem =
  | DirectMessageNotificationItem
  | MentionNotificationItem
  | ReplyNotificationItem
  | RoomMessageNotificationItem;

export type NotificationPage = {
  items: NotificationItem[];
  totalCount: number;
  hasMore: boolean;
  serverName: string | null;
};

export function createNotificationAPI(config: NotificationAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(NotificationService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  return {
    async listNotifications(limit = 50, offset = 0): Promise<NotificationPage> {
      return notificationPage(
        await client.listNotifications({ page: { limit, offset } }, { headers: headers() })
      );
    },

    async listRoomNotifications(roomId: string, limit = 1, offset = 0): Promise<NotificationPage> {
      return notificationPage(
        await client.listRoomNotifications({ roomId, page: { limit, offset } }, { headers: headers() })
      );
    },

    async hasNotifications(): Promise<boolean> {
      return (await client.hasNotifications({}, { headers: headers() })).hasNotifications;
    },

    async listNotificationCounts(): Promise<Record<string, number>> {
      const response = await client.listNotificationCounts({}, { headers: headers() });
      return Object.fromEntries(
        response.roomCounts.map((count) => [count.roomId, count.totalCount] as const)
      );
    },

    async dismissNotification(notificationId: string): Promise<boolean> {
      return (await client.dismissNotification({ notificationId }, { headers: headers() }))
        .dismissed;
    },

    async dismissAllNotifications(): Promise<number> {
      return (await client.dismissAllNotifications({}, { headers: headers() })).dismissedCount;
    }
  };
}

export type NotificationAPI = ReturnType<typeof createNotificationAPI>;

function notificationPage(response: ListNotificationsResponse | ListRoomNotificationsResponse): NotificationPage {
  return {
    items: response.items.flatMap((item) => {
      const mapped = notificationItem(item);
      return mapped ? [mapped] : [];
    }),
    totalCount: Number(response.page?.totalCount ?? 0),
    hasMore: response.page?.hasMore ?? false,
    serverName: response.serverName || null
  };
}

function notificationItem(item: APINotificationItem): NotificationItem | null {
  const base = {
    id: item.id,
    createdAt: item.createdAt?.toDate().toISOString() ?? new Date(0).toISOString(),
    actor: notificationActor(item.actor),
    summary: item.summary
  };

  switch (item.kind.case) {
    case 'directMessage':
      return {
        kind: NotificationItemKind.DirectMessage,
        ...base,
        room: { id: item.kind.value.roomId }
      };
    case 'mention':
      return {
        kind: NotificationItemKind.Mention,
        ...base,
        mentionRoom: item.kind.value.room
          ? { id: item.kind.value.room.id, name: item.kind.value.room.name }
          : null,
        mentionEventId: item.kind.value.eventId,
        mentionInThread: item.kind.value.threadRootEventId ?? null
      };
    case 'reply':
      return {
        kind: NotificationItemKind.Reply,
        ...base,
        replyRoom: item.kind.value.room
          ? { id: item.kind.value.room.id, name: item.kind.value.room.name }
          : null,
        replyEventId: item.kind.value.eventId,
        inReplyToId: item.kind.value.inReplyToId,
        replyInThread: item.kind.value.threadRootEventId ?? null
      };
    case 'roomMessage':
      return {
        kind: NotificationItemKind.RoomMessage,
        ...base,
        roomMsgRoom: item.kind.value.room
          ? { id: item.kind.value.room.id, name: item.kind.value.room.name }
          : null,
        roomMsgEventId: item.kind.value.eventId
      };
    default:
      return null;
  }
}

function notificationActor(actor: APIUserPresenceSummary | undefined): NotificationActor | null {
  const summary = actor?.user;
  if (!actor || !summary) return null;
  return {
    id: summary.id,
    login: summary.login,
    displayName: summary.displayName,
    deleted: summary.deleted,
    avatarUrl: summary.avatarUrl ?? null,
    presenceStatus: apiPresenceStatus(actor.presenceStatus),
    customStatus: actor.customStatus
      ? {
          emoji: actor.customStatus.emoji,
          text: actor.customStatus.text,
          expiresAt: actor.customStatus.expiresAt?.toDate().toISOString() ?? null
        }
      : null
  };
}

function apiPresenceStatus(status: APIPresenceStatus): PresenceStatus {
  switch (status) {
    case APIPresenceStatus.AWAY:
      return PresenceStatus.Away;
    case APIPresenceStatus.DO_NOT_DISTURB:
      return PresenceStatus.DoNotDisturb;
    case APIPresenceStatus.ONLINE:
      return PresenceStatus.Online;
    case APIPresenceStatus.OFFLINE:
    case APIPresenceStatus.UNSPECIFIED:
    default:
      return PresenceStatus.Offline;
  }
}
