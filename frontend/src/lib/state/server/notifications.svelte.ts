import { SvelteSet } from 'svelte/reactivity';
import { resolve } from '$app/paths';
import {
  DismissNotificationRequest,
  ListNotificationsRequest,
  NotificationKind,
  type NotificationItemView
} from '$lib/pb/chatto/api/v1/chat_pb';
import type { User } from '$lib/pb/chatto/core/v1/models_pb';
import type { UserAvatarUserFragment } from '$lib/chatTypes';
import { serverIdToSegment } from '$lib/navigation';
import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
import type { WireClient } from '$lib/wire/client';

export type NotificationWireClient = Pick<
  WireClient,
  'listNotifications' | 'hasNotifications' | 'dismissNotification' | 'dismissAllNotifications'
>;

type NotificationBase = {
  id: string;
  createdAt: string;
  actor: UserAvatarUserFragment | null;
  summary: string;
};

export type DMMessageNotificationItem = NotificationBase & {
  __typename: 'DMMessageNotificationItem';
  room: { id: string };
};

export type MentionNotificationItem = NotificationBase & {
  __typename: 'MentionNotificationItem';
  mentionRoom: { id: string; name: string } | null;
  mentionEventId: string | null;
  mentionInThread: string | null;
};

export type ReplyNotificationItem = NotificationBase & {
  __typename: 'ReplyNotificationItem';
  replyRoom: { id: string; name: string } | null;
  replyEventId: string | null;
  inReplyToId: string | null;
  replyInThread: string | null;
};

export type RoomMessageNotificationItem = NotificationBase & {
  __typename: 'RoomMessageNotificationItem';
  roomMsgRoom: { id: string; name: string } | null;
  roomMsgEventId: string | null;
};

export type NotificationItem =
  | DMMessageNotificationItem
  | MentionNotificationItem
  | ReplyNotificationItem
  | RoomMessageNotificationItem;

/**
 * Normalized view of a notification's target (where it points to in the app).
 * Avoids `__typename` switches at every read site — see {@link notificationTarget}.
 */
export type NotificationTarget = {
  isDM: boolean;
  spaceName: string | null;
  roomId: string | null;
  roomName: string | null;
  eventId: string | null;
  /** Thread root event ID for thread-reply notifications; null otherwise. */
  threadRootId: string | null;
};

/**
 * Extract the target a notification points to. Adding a new notification type
 * means updating this single function instead of every read site.
 */
export function notificationTarget(n: NotificationItem): NotificationTarget {
  switch (n.__typename) {
    case 'DMMessageNotificationItem':
      return {
        isDM: true,
        spaceName: null,
        roomId: n.room.id,
        roomName: null,
        eventId: null,
        threadRootId: null
      };
    case 'MentionNotificationItem':
      return {
        isDM: false,
        spaceName: null,
        roomId: n.mentionRoom?.id ?? null,
        roomName: n.mentionRoom?.name ?? null,
        eventId: n.mentionEventId ?? null,
        threadRootId: n.mentionInThread ?? null
      };
    case 'ReplyNotificationItem':
      return {
        isDM: false,
        spaceName: null,
        roomId: n.replyRoom?.id ?? null,
        roomName: n.replyRoom?.name ?? null,
        eventId: n.replyEventId ?? null,
        threadRootId: n.replyInThread ?? null
      };
    case 'RoomMessageNotificationItem':
      return {
        isDM: false,
        spaceName: null,
        roomId: n.roomMsgRoom?.id ?? null,
        roomName: n.roomMsgRoom?.name ?? null,
        eventId: n.roomMsgEventId ?? null,
        threadRootId: null
      };
  }
}

/**
 * Notification state store.
 * Manages notifications for the current user with real-time sync.
 */
export class NotificationStore {
  notifications = $state<NotificationItem[]>([]);
  /**
   * Server display name, captured alongside the notification list and used
   * by getLocationString() for non-DM notifications.
   */
  serverName = $state<string | null>(null);
  loading = $state(false);
  error = $state<string | null>(null);

  constructor(
    private readonly serverId: string,
    private readonly getWireClient: () => NotificationWireClient | undefined = () =>
      wireEventBusManager.getClient(serverId)
  ) {}

  // Derived properties
  get hasNotifications() {
    return this.notifications.length > 0;
  }

  get count() {
    return this.notifications.length;
  }

  /**
   * Get the set of thread root IDs that have pending reply notifications.
   * Used to show notification indicators on thread buttons.
   */
  get threadsWithNotifications(): SvelteSet<string> {
    const threadIds = new SvelteSet<string>();
    for (const n of this.notifications) {
      if (n.__typename === 'ReplyNotificationItem' && n.replyInThread) {
        threadIds.add(n.replyInThread);
      }
    }
    return threadIds;
  }

  /**
   * Check if a specific thread has pending notifications.
   */
  hasThreadNotification(threadRootId: string): boolean {
    return this.notifications.some(
      (n) => n.__typename === 'ReplyNotificationItem' && n.replyInThread === threadRootId
    );
  }

  /**
   * Check if a specific room has pending non-DM notifications.
   */
  hasRoomNotification(roomId: string): boolean {
    return this.notifications.some((n) => {
      const t = notificationTarget(n);
      return !t.isDM && t.roomId === roomId;
    });
  }

  /**
   * Check if the server has any pending notifications.
   *
   * Post-PR(b) the API surface has only one server, so this collapses to
   * "any non-DM notification exists." The signature keeps a `_spaceId`
   * parameter for call-site compatibility — it's ignored.
   */
  hasSpaceNotification(_spaceId?: string): boolean {
    return this.notifications.some((n) => !notificationTarget(n).isDM);
  }

  /**
   * Get the most recent server notification.
   * Notifications are sorted most-recent-first, so .find returns the freshest.
   */
  getSpaceNotification(_spaceId?: string): NotificationItem | undefined {
    return this.notifications.find((n) => !notificationTarget(n).isDM);
  }

  /**
   * Get the most recent non-DM notification for a room.
   */
  getRoomNotification(roomId: string): NotificationItem | undefined {
    return this.notifications.find((n) => {
      const t = notificationTarget(n);
      return !t.isDM && t.roomId === roomId;
    });
  }

  /**
   * Check if there are any pending DM notifications.
   */
  hasDMNotifications(): boolean {
    return this.notifications.some((n) => n.__typename === 'DMMessageNotificationItem');
  }

  /**
   * Get the most recent DM notification.
   * Returns undefined if no DM notifications exist.
   */
  getDMNotification(): NotificationItem | undefined {
    return this.notifications.find((n) => n.__typename === 'DMMessageNotificationItem');
  }

  /**
   * Check if a specific DM conversation has pending notifications.
   * Counterpart to {@link hasRoomNotification}, which excludes DMs.
   */
  hasDMRoomNotification(roomId: string): boolean {
    return this.notifications.some(
      (n) => n.__typename === 'DMMessageNotificationItem' && n.room.id === roomId
    );
  }

  /**
   * Get the most recent notification for a DM conversation.
   */
  getDMRoomNotification(roomId: string): NotificationItem | undefined {
    return this.notifications.find(
      (n) => n.__typename === 'DMMessageNotificationItem' && n.room.id === roomId
    );
  }

  /**
   * Dismiss all thread-scoped notifications (replies + mentions) for a thread.
   * Called when a user opens a thread to clear the notification indicator.
   */
  async dismissThreadNotifications(threadRootId: string): Promise<void> {
    const threadNotifications = this.notifications.filter(
      (n) =>
        (n.__typename === 'ReplyNotificationItem' && n.replyInThread === threadRootId) ||
        (n.__typename === 'MentionNotificationItem' && n.mentionInThread === threadRootId)
    );

    await Promise.all(threadNotifications.map((n) => this.dismiss(n.id)));
  }

  /**
   * Dismiss room-level mention notifications for a specific room.
   * Called when a user enters a room. Thread-scoped mentions are NOT dismissed
   * here — they're dismissed when the user opens the specific thread (via
   * dismissThreadNotifications), matching the symmetry with reply notifications.
   */
  async dismissMentionNotifications(roomId: string): Promise<void> {
    const mentionNotifications = this.notifications.filter(
      (n) =>
        n.__typename === 'MentionNotificationItem' &&
        !n.mentionInThread &&
        n.mentionRoom?.id === roomId
    );

    await Promise.all(mentionNotifications.map((n) => this.dismiss(n.id)));
  }

  /**
   * Dismiss room-level reply notifications for a specific room.
   * Called when a user enters a room to clear reply notification indicators.
   * Only dismisses room-level replies (not thread replies, which are dismissed
   * separately when opening the specific thread via dismissThreadNotifications).
   */
  async dismissRoomReplyNotifications(roomId: string): Promise<void> {
    const roomReplyNotifications = this.notifications.filter(
      (n) =>
        n.__typename === 'ReplyNotificationItem' && !n.replyInThread && n.replyRoom?.id === roomId
    );

    await Promise.all(roomReplyNotifications.map((n) => this.dismiss(n.id)));
  }

  /**
   * Dismiss all room message notifications for a specific room.
   * Called when a user enters a room to clear "all messages" notification indicators.
   */
  async dismissRoomMessageNotifications(roomId: string): Promise<void> {
    const roomMsgNotifications = this.notifications.filter(
      (n) => n.__typename === 'RoomMessageNotificationItem' && n.roomMsgRoom?.id === roomId
    );

    await Promise.all(roomMsgNotifications.map((n) => this.dismiss(n.id)));
  }

  /**
   * Dismiss all DM notifications for a specific conversation.
   * Called when a user enters a DM conversation to clear notification indicators.
   */
  async dismissDMNotifications(roomId: string): Promise<void> {
    const dmNotifications = this.notifications.filter(
      (n) => n.__typename === 'DMMessageNotificationItem' && n.room.id === roomId
    );

    await Promise.all(dmNotifications.map((n) => this.dismiss(n.id)));
  }

  /**
   * Fetch all notifications from the server.
   *
   * Resilience contract: a server-side error (e.g. an older backend without
   * this wire method, network failure, transient 500) records the error
   * message and logs it, but leaves `this.notifications` at its previous value.
   */
  async fetch() {
    this.loading = true;
    this.error = null;

    try {
      const client = this.getWireClient();
      if (!client) {
        console.warn(
          `[server:${this.serverId}] cannot fetch notifications before wire client is ready`
        );
        return;
      }

      const response = await client.listNotifications(new ListNotificationsRequest({ limit: 50 }));
      this.notifications = response.items.map(notificationItemFromWire).filter(isNotificationItem);
      this.serverName = response.serverName || null;
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'Failed to fetch notifications';
      console.error('Failed to fetch notifications:', e);
    } finally {
      this.loading = false;
    }
  }

  /**
   * Check if user has any notifications (lightweight check for bell icon).
   */
  async checkHasNotifications(): Promise<boolean> {
    try {
      const client = this.getWireClient();
      if (!client) return false;
      const result = await client.hasNotifications();
      return result.hasNotifications;
    } catch (e) {
      console.error('Failed to check notifications:', e);
      return false;
    }
  }

  /**
   * Dismiss a single notification. Optimistic: removes locally first, rolls
   * back on failure. The orange dot disappears the moment the user clicks.
   */
  async dismiss(notificationId: string): Promise<boolean> {
    const removed = this.notifications.find((n) => n.id === notificationId);
    if (!removed) return false;

    this.notifications = this.notifications.filter((n) => n.id !== notificationId);

    try {
      const client = this.getWireClient();
      if (!client) {
        this.#restoreNotification(removed);
        return false;
      }

      const result = await client.dismissNotification(
        new DismissNotificationRequest({ notificationId })
      );
      if (!result.dismissed) {
        this.#restoreNotification(removed);
        return false;
      }
      return true;
    } catch (e) {
      console.error('Failed to dismiss notification:', e);
      this.#restoreNotification(removed);
      return false;
    }
  }

  /**
   * Dismiss all notifications. Optimistic: clears locally first, rolls back
   * on failure.
   */
  async dismissAll(): Promise<number> {
    const original = this.notifications;
    if (original.length === 0) return 0;

    this.notifications = [];

    try {
      const client = this.getWireClient();
      if (!client) {
        this.notifications = original;
        return 0;
      }

      const result = await client.dismissAllNotifications();
      return result.dismissedCount;
    } catch (e) {
      console.error('Failed to dismiss all notifications:', e);
      this.notifications = original;
      return 0;
    }
  }

  /**
   * Re-insert a previously-removed notification, sorted most-recent-first by
   * createdAt to preserve the canonical ordering after a rollback.
   */
  #restoreNotification(notification: NotificationItem): void {
    this.notifications = [...this.notifications, notification].sort((a, b) =>
      b.createdAt.localeCompare(a.createdAt)
    );
  }

  /**
   * Add a notification (for real-time updates from instance events).
   * Triggers a refetch to get full notification data.
   */
  async addNotification() {
    await this.fetch();
  }

  /**
   * Remove a notification by ID (for cross-device sync).
   */
  removeNotification(notificationId: string) {
    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
  }

  /**
   * Get location string for a notification (e.g., "#general in My Server").
   * Returns null for DM notifications and any notification missing names.
   * The "in <name>" suffix uses the instance display name.
   */
  getLocationString(notification: NotificationItem): string | null {
    const t = notificationTarget(notification);
    if (t.isDM || !t.roomName) return null;
    const serverName = this.serverName;
    if (!serverName) return `#${t.roomName}`;
    return `#${t.roomName} in ${serverName}`;
  }

  /**
   * Build a clean (no `?highlight=`) destination path for a notification.
   * Use this with `PendingHighlightStore.set()` to deliver the highlight
   * intent without polluting the URL.
   */
  getCleanPath(serverId: string, notification: NotificationItem): string {
    const seg = serverIdToSegment(serverId);
    const t = notificationTarget(notification);

    if (t.isDM && t.roomId) {
      return resolve('/chat/[serverId]/[roomId]', {
        serverId: seg,
        roomId: t.roomId
      });
    }
    if (!t.roomId) {
      return resolve('/chat/[serverId]', { serverId: seg });
    }
    if (t.threadRootId) {
      return resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: seg,
        roomId: t.roomId,
        threadId: t.threadRootId
      });
    }
    return resolve('/chat/[serverId]/[roomId]', {
      serverId: seg,
      roomId: t.roomId
    });
  }

  /**
   * Get navigation info for a notification.
   * Returns the path to navigate to when acting on the notification, with
   * `?highlight=<eventId>` for messages.
   *
   * @deprecated Prefer `getCleanPath` + `PendingHighlightStore.set`. The
   *   `?highlight=` URL param survives refresh and re-fires; the transient
   *   store delivers the intent one-shot. Kept for permalink-style call sites
   *   that genuinely want the URL to encode the highlight.
   */
  getNavigationPath(serverId: string, notification: NotificationItem): string {
    const seg = serverIdToSegment(serverId);
    const t = notificationTarget(notification);

    if (t.isDM && t.roomId) {
      return resolve('/chat/[serverId]/[roomId]', {
        serverId: seg,
        roomId: t.roomId
      });
    }

    if (!t.roomId) {
      return resolve('/chat/[serverId]', { serverId: seg });
    }

    if (t.threadRootId && t.eventId) {
      return (
        resolve('/chat/[serverId]/[roomId]/[threadId]', {
          serverId: seg,
          roomId: t.roomId,
          threadId: t.threadRootId
        }) +
        '?highlight=' +
        t.eventId
      );
    }

    const roomPath = resolve('/chat/[serverId]/[roomId]', {
      serverId: seg,
      roomId: t.roomId
    });
    return t.eventId ? `${roomPath}?highlight=${t.eventId}` : roomPath;
  }
}

function notificationItemFromWire(view: NotificationItemView): NotificationItem | null {
  const base = notificationBaseFromWire(view);
  const room = roomReferenceFromWire(view);

  switch (view.kind) {
    case NotificationKind.DM_MESSAGE:
      if (!view.roomId) return null;
      return {
        ...base,
        __typename: 'DMMessageNotificationItem',
        room: { id: view.roomId }
      };
    case NotificationKind.MENTION:
      return {
        ...base,
        __typename: 'MentionNotificationItem',
        mentionRoom: room,
        mentionEventId: view.eventId ?? null,
        mentionInThread: view.threadRootEventId ?? null
      };
    case NotificationKind.REPLY:
      return {
        ...base,
        __typename: 'ReplyNotificationItem',
        replyRoom: room,
        replyEventId: view.eventId ?? null,
        inReplyToId: view.inReplyToId ?? null,
        replyInThread: view.threadRootEventId ?? null
      };
    case NotificationKind.ROOM_MESSAGE:
      return {
        ...base,
        __typename: 'RoomMessageNotificationItem',
        roomMsgRoom: room,
        roomMsgEventId: view.eventId ?? null
      };
    case NotificationKind.UNSPECIFIED:
      return null;
  }
}

function notificationBaseFromWire(view: NotificationItemView): NotificationBase {
  return {
    id: view.id,
    createdAt: view.createdAt?.toDate().toISOString() ?? new Date(0).toISOString(),
    actor: userToAvatarFragment(view.actor),
    summary: view.summary || 'New notification'
  };
}

function roomReferenceFromWire(view: NotificationItemView): { id: string; name: string } | null {
  if (!view.roomId) return null;
  return {
    id: view.roomId,
    name: view.roomName
  };
}

function userToAvatarFragment(user: User | undefined): UserAvatarUserFragment | null {
  if (!user) return null;
  return {
    __typename: 'User',
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: null,
    presenceStatus: 'OFFLINE' as UserAvatarUserFragment['presenceStatus']
  };
}

function isNotificationItem(value: NotificationItem | null): value is NotificationItem {
  return value !== null;
}
