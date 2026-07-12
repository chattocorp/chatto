import { authHeaders, Code, ConnectError, createChattoClient } from './connect.js';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import type {
  ListRoomNotificationsResponse,
  ListNotificationsResponse,
  NotificationItem as APINotificationItem
} from '@chatto/api-types/api/v1/notifications_pb';
import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { PresenceStatus } from './renderTypes.js';

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
};

export function createNotificationAPI(config: NotificationAPIConfig) {
  const client = createChattoClient(NotificationService, config);
  const headers = () => authHeaders(config);
  const listRoomNotificationCounts = async (): Promise<Record<string, number>> => {
    const response = await client.listRoomNotificationCounts({}, { headers: headers() });
    return Object.fromEntries(
      response.roomCounts.map((count) => [count.roomId, count.totalCount] as const)
    );
  };

  return {
    async listNotifications(limit = 50, offset = 0): Promise<NotificationPage> {
      return notificationPage(
        await client.listNotifications({ page: { limit, offset } }, { headers: headers() })
      );
    },

    async listRoomNotifications(roomId: string, limit = 1, offset = 0): Promise<NotificationPage> {
      return notificationPage(
        await client.listRoomNotifications(
          { roomId, page: { limit, offset } },
          { headers: headers() }
        )
      );
    },

    async getNotification(notificationId: string): Promise<NotificationItem | null> {
      try {
        const response = await client.getNotification({ notificationId }, { headers: headers() });
        return response.notification ? notificationItem(response.notification) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetNotifications(notificationIds: string[]): Promise<NotificationItem[]> {
      const response = await client.batchGetNotifications(
        { notificationIds },
        { headers: headers() }
      );
      return response.notifications.flatMap((item) => {
        const mapped = notificationItem(item);
        return mapped ? [mapped] : [];
      });
    },

    async hasNotifications(): Promise<boolean> {
      return (await client.hasNotifications({}, { headers: headers() })).hasNotifications;
    },

    async listRoomNotificationCounts(): Promise<Record<string, number>> {
      return listRoomNotificationCounts();
    },

    async listNotificationCounts(): Promise<Record<string, number>> {
      return listRoomNotificationCounts();
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

function notificationPage(
  response: ListNotificationsResponse | ListRoomNotificationsResponse
): NotificationPage {
  return {
    items: response.notifications.flatMap((item) => {
      const mapped = notificationItem(item);
      return mapped ? [mapped] : [];
    }),
    totalCount: Number(response.page?.totalCount ?? 0),
    hasMore: response.page?.hasMore ?? false
  };
}

function notificationItem(item: APINotificationItem): NotificationItem | null {
  const actor = notificationActor(item.actor);
  const base = {
    id: item.id,
    createdAt: item.createdAt?.toDate().toISOString() ?? new Date(0).toISOString(),
    actor
  };

  switch (item.kind.case) {
    case 'directMessage':
      return {
        kind: NotificationItemKind.DirectMessage,
        ...base,
        summary: notificationSummary(actor, NotificationItemKind.DirectMessage),
        room: { id: item.kind.value.room?.id ?? '' }
      };
    case 'mention':
      return {
        kind: NotificationItemKind.Mention,
        ...base,
        summary: notificationSummary(actor, NotificationItemKind.Mention),
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
        summary: notificationSummary(actor, NotificationItemKind.Reply),
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
        summary: notificationSummary(actor, NotificationItemKind.RoomMessage),
        roomMsgRoom: item.kind.value.room
          ? { id: item.kind.value.room.id, name: item.kind.value.room.name }
          : null,
        roomMsgEventId: item.kind.value.eventId
      };
    default:
      return null;
  }
}

function notificationSummary(actor: NotificationActor | null, kind: NotificationItemKind): string {
  const actorName = actor?.displayName || null;
  switch (kind) {
    case NotificationItemKind.DirectMessage:
      return actorName ? `${actorName} sent you a message` : 'New message';
    case NotificationItemKind.Mention:
      return actorName ? `${actorName} mentioned you` : 'You were mentioned';
    case NotificationItemKind.Reply:
      return actorName ? `${actorName} replied to your message` : 'New reply to your message';
    case NotificationItemKind.RoomMessage:
      return actorName ? `${actorName} posted a message` : 'New message';
  }
}

function notificationActor(actor: APIUser | undefined): NotificationActor | null {
  if (!actor) return null;
  return {
    id: actor.id,
    login: actor.login,
    displayName: actor.displayName,
    deleted: actor.deleted,
    avatarUrl: actor.avatarUrl ?? null,
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
