import { SvelteSet } from 'svelte/reactivity';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import {
  NotificationItemKind,
  type DirectMessageNotificationItem,
  type MentionNotificationItem,
  type NotificationAPI,
  type NotificationItem,
  type ReplyNotificationItem,
  type RoomMessageNotificationItem
} from '$lib/api-client/notifications';

// Union type for all notification types
export type { NotificationItem };

/**
 * Normalized view of a notification's target (where it points to in the app).
 * Avoids discriminant switches at every read site — see {@link notificationTarget}.
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

export type RoomNotificationLookup = {
  ok: boolean;
  totalCount: number | null;
  notification: NotificationItem | null;
};

export type RoomNotificationResolveOptions = {
  isDM?: boolean;
};

function isDMNotification(
  notification: NotificationItem
): notification is DirectMessageNotificationItem {
  return notification.kind === NotificationItemKind.DirectMessage;
}

function isMentionNotification(
  notification: NotificationItem
): notification is MentionNotificationItem {
  return notification.kind === NotificationItemKind.Mention;
}

function isReplyNotification(
  notification: NotificationItem
): notification is ReplyNotificationItem {
  return notification.kind === NotificationItemKind.Reply;
}

function isRoomMessageNotification(
  notification: NotificationItem
): notification is RoomMessageNotificationItem {
  return notification.kind === NotificationItemKind.RoomMessage;
}

/**
 * Extract the target a notification points to. Adding a new notification type
 * means updating this single function instead of every read site.
 */
export function notificationTarget(n: NotificationItem): NotificationTarget {
  if (isDMNotification(n)) {
    return {
      isDM: true,
      spaceName: null,
      roomId: n.room.id,
      roomName: null,
      eventId: null,
      threadRootId: null
    };
  }
  if (isMentionNotification(n)) {
    return {
      isDM: false,
      spaceName: null,
      roomId: n.mentionRoom?.id ?? null,
      roomName: n.mentionRoom?.name ?? null,
      eventId: n.mentionEventId ?? null,
      threadRootId: n.mentionInThread ?? null
    };
  }
  if (isReplyNotification(n)) {
    return {
      isDM: false,
      spaceName: null,
      roomId: n.replyRoom?.id ?? null,
      roomName: n.replyRoom?.name ?? null,
      eventId: n.replyEventId ?? null,
      threadRootId: n.replyInThread ?? null
    };
  }
  if (isRoomMessageNotification(n)) {
    return {
      isDM: false,
      spaceName: null,
      roomId: n.roomMsgRoom?.id ?? null,
      roomName: n.roomMsgRoom?.name ?? null,
      eventId: n.roomMsgEventId ?? null,
      threadRootId: null
    };
  }
  return {
    isDM: false,
    spaceName: null,
    roomId: null,
    roomName: null,
    eventId: null,
    threadRootId: null
  };
}

/**
 * Notification state store.
 * Manages notifications for the current user with real-time sync.
 */
export class NotificationStore {
  #api: NotificationAPI;
  #locallyDismissedNotificationIds = new SvelteSet<string>();
  notifications = $state<NotificationItem[]>([]);
  unreadNotificationCount = $state(0);
  loading = $state(false);
  hasLoaded = $state(false);
  error = $state<string | null>(null);

  constructor(api: NotificationAPI) {
    this.#api = api;
  }

  // Derived properties
  get hasNotifications() {
    return this.notifications.length > 0;
  }

  get count() {
    return this.notifications.length;
  }

  setUnreadNotificationCount(count: number): void {
    this.unreadNotificationCount = Math.max(0, count);
  }

  /**
   * Get the set of thread root IDs that have pending reply notifications.
   * Used to show notification indicators on thread buttons.
   */
  get threadsWithNotifications(): SvelteSet<string> {
    const threadIds = new SvelteSet<string>();
    for (const n of this.notifications) {
      if (isReplyNotification(n) && n.replyInThread) {
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
      (n) => isReplyNotification(n) && n.replyInThread === threadRootId
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
    return this.notifications.some((n) => isDMNotification(n));
  }

  /**
   * Get the most recent DM notification.
   * Returns undefined if no DM notifications exist.
   */
  getDMNotification(): NotificationItem | undefined {
    return this.notifications.find((n) => isDMNotification(n));
  }

  /**
   * Check if a specific DM conversation has pending notifications.
   * Counterpart to {@link hasRoomNotification}, which excludes DMs.
   */
  hasDMRoomNotification(roomId: string): boolean {
    return this.notifications.some((n) => isDMNotification(n) && n.room.id === roomId);
  }

  /**
   * Get the most recent notification for a DM conversation.
   */
  getDMRoomNotification(roomId: string): NotificationItem | undefined {
    return this.notifications.find((n) => isDMNotification(n) && n.room.id === roomId);
  }

  getCachedRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): NotificationItem | undefined {
    return options.isDM ? this.getDMRoomNotification(roomId) : this.getRoomNotification(roomId);
  }

  /**
   * Fetch all notifications from the server.
   *
   * Resilience contract: a server-side error (e.g. a schema mismatch on a
   * remote instance running an older backend, network failure, transient
   * 500) records the error message and logs it, but leaves
   * `this.notifications` at its previous value. This matters in
   * multi-instance setups — the bell, DM dot, etc. aggregate across
   * NotificationStore instances, and one bad response on one instance
   * must not erase already-loaded notifications on others.
   */
  async fetch() {
    this.loading = true;
    this.error = null;

    try {
      const page = await this.#api.listNotifications(50);
      this.notifications = page.items;
      this.unreadNotificationCount = page.totalCount;
      this.hasLoaded = true;
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'Failed to fetch notifications';
      console.error('Failed to fetch notifications:', e);
    } finally {
      this.loading = false;
    }
  }

  /**
   * Fetch the newest pending notification for a single room.
   *
   * Room sidebar badge clicks need the same scoped source as room notification
   * counts when the global cached page is empty, stale, or does not include
   * this room's notification.
   */
  async fetchRoomNotification(roomId: string): Promise<RoomNotificationLookup> {
    try {
      const page = await this.#api.listRoomNotifications(roomId, 1);
      const notification = page.items[0] ?? null;
      if (notification) {
        this.#upsertNotification(notification);
      }

      return {
        ok: true,
        totalCount: page.totalCount,
        notification
      };
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'Failed to fetch room notification';
      console.error('Failed to fetch room notification:', e);
      return { ok: false, totalCount: null, notification: null };
    }
  }

  async resolveRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): Promise<RoomNotificationLookup> {
    const cached = this.getCachedRoomNotification(roomId, options);
    if (cached) {
      return { ok: true, totalCount: null, notification: cached };
    }
    return this.fetchRoomNotification(roomId);
  }

  /**
   * Check if user has any notifications (lightweight check for bell icon).
   */
  async checkHasNotifications(): Promise<boolean> {
    try {
      return await this.#api.hasNotifications();
    } catch (e) {
      console.error('Failed to check notifications:', e);
      return false;
    }
  }

  /**
   * Dismiss a single notification. Optimistic: removes locally first, rolls
   * back on failure. The notification indicator disappears the moment the user clicks.
   */
  async dismiss(notificationId: string): Promise<boolean> {
    const removed = this.notifications.find((n) => n.id === notificationId);
    if (!removed) return false;

    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
    this.unreadNotificationCount = Math.max(0, this.unreadNotificationCount - 1);
    this.#markLocalDismissal(notificationId);

    try {
      if (!(await this.#api.dismissNotification(notificationId))) {
        this.#locallyDismissedNotificationIds.delete(notificationId);
        this.#restoreNotification(removed);
        this.unreadNotificationCount += 1;
        return false;
      }
      return true;
    } catch (e) {
      console.error('Failed to dismiss notification:', e);
      this.#locallyDismissedNotificationIds.delete(notificationId);
      this.#restoreNotification(removed);
      this.unreadNotificationCount += 1;
      return false;
    }
  }

  /**
   * Dismiss all notifications. Optimistic: clears locally first, rolls back
   * on failure.
   */
  async dismissAll(): Promise<number> {
    const original = this.notifications;
    const originalCount = this.unreadNotificationCount;
    if (original.length === 0) return 0;

    this.notifications = [];
    this.unreadNotificationCount = 0;
    for (const notification of original) {
      this.#markLocalDismissal(notification.id);
    }

    try {
      return await this.#api.dismissAllNotifications();
    } catch (e) {
      console.error('Failed to dismiss all notifications:', e);
      for (const notification of original) {
        this.#locallyDismissedNotificationIds.delete(notification.id);
      }
      this.notifications = original;
      this.unreadNotificationCount = originalCount;
      return 0;
    }
  }

  /**
   * Re-insert a previously-removed notification, sorted most-recent-first by
   * createdAt to preserve the canonical ordering after a rollback.
   */
  #restoreNotification(notification: NotificationItem): void {
    this.#upsertNotification(notification);
  }

  #upsertNotification(notification: NotificationItem): void {
    this.notifications = [
      ...this.notifications.filter((n) => n.id !== notification.id),
      notification
    ].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  }

  #markLocalDismissal(notificationId: string): void {
    this.#locallyDismissedNotificationIds.add(notificationId);
    const timeout = setTimeout(
      () => this.#locallyDismissedNotificationIds.delete(notificationId),
      30_000
    );
    if (typeof timeout === 'object' && timeout !== null && 'unref' in timeout) {
      (timeout as { unref: () => void }).unref();
    }
  }

  /**
   * Add a notification (for real-time updates from instance events).
   * Triggers a refetch to get full notification data.
   */
  async addNotification() {
    // Refetch to get the new notification with full data
    await this.fetch();
  }

  /**
   * Remove a notification by ID (for cross-device sync).
   */
  removeNotification(notificationId: string) {
    const removed = this.notifications.find((n) => n.id === notificationId);
    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
    if (removed) {
      this.unreadNotificationCount = Math.max(0, this.unreadNotificationCount - 1);
    }
    return removed ? notificationTarget(removed).roomId : null;
  }

  consumeLocalDismissal(notificationId: string): boolean {
    const local = this.#locallyDismissedNotificationIds.has(notificationId);
    this.#locallyDismissedNotificationIds.delete(notificationId);
    return local;
  }

  /**
   * Get location string for a notification (e.g., "#general in My Server").
   * Returns null for DM notifications and any notification missing names.
   * The "in <name>" suffix uses the connected instance display name supplied
   * by the caller.
   */
  getLocationString(notification: NotificationItem, serverName?: string | null): string | null {
    const t = notificationTarget(notification);
    if (t.isDM || !t.roomName) return null;
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
      // DMs are now rooms on the Server (#330 phase 3) — use the standard
      // room URL rather than the legacy /chat/dm/... path.
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
      // DMs are now rooms on the Server (#330 phase 3) — use the standard
      // room URL rather than the legacy /chat/dm/... path.
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
