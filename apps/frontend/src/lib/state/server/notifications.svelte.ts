import { SvelteSet } from 'svelte/reactivity';
import { graphql, useFragment } from '$lib/gql';
import {
  NotificationItemViewFragmentDoc,
  type NotificationItemViewFragment
} from '$lib/gql/graphql';
import type { Client } from '@urql/svelte';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';

// GraphQL queries and mutations
const NotificationItemViewFragment = graphql(`
  fragment NotificationItemView on NotificationItem {
    __typename
    ... on DMMessageNotificationItem {
      id
      createdAt
      actor {
        ...UserAvatarUser
      }
      summary
      room {
        id
      }
    }
    ... on MentionNotificationItem {
      id
      createdAt
      actor {
        ...UserAvatarUser
      }
      summary
      mentionRoom: room {
        id
        name
      }
      mentionEventId: eventId
      mentionInThread: threadRootEventId
    }
    ... on ReplyNotificationItem {
      id
      createdAt
      actor {
        ...UserAvatarUser
      }
      summary
      replyRoom: room {
        id
        name
      }
      replyEventId: eventId
      inReplyToId
      replyInThread: threadRootEventId
    }
    ... on RoomMessageNotificationItem {
      id
      createdAt
      actor {
        ...UserAvatarUser
      }
      summary
      roomMsgRoom: room {
        id
        name
      }
      roomMsgEventId: eventId
    }
  }
`);

const NotificationsQueryDoc = graphql(`
  query Notifications {
    viewer {
      notifications(limit: 50) {
        totalCount
        items {
          ...NotificationItemView
        }
      }
    }
  }
`);

const RoomNotificationQueryDoc = graphql(`
  query RoomNotification($roomId: ID!) {
    room(roomId: $roomId) {
      viewerNotifications(limit: 1) {
        totalCount
        items {
          ...NotificationItemView
        }
      }
    }
  }
`);

const HasNotificationsQueryDoc = graphql(`
  query HasNotifications {
    viewer {
      hasNotifications
    }
  }
`);

const InstanceNameQueryDoc = graphql(`
  query NotificationInstanceName {
    server {
      profile {
        name
      }
    }
  }
`);

const DismissNotificationMutationDoc = graphql(`
  mutation DismissNotification($input: DismissNotificationInput!) {
    dismissNotification(input: $input)
  }
`);

const DismissAllNotificationsMutationDoc = graphql(`
  mutation DismissAllNotifications {
    dismissAllNotifications
  }
`);

// Union type for all notification types
export type NotificationItem = NotificationItemViewFragment;

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

export type NotificationDismissalCounts = {
  total: number;
  byRoom: Record<string, number>;
};

export type RoomNotificationLookup = {
  ok: boolean;
  totalCount: number | null;
  notification: NotificationItem | null;
};

export type RoomNotificationResolveOptions = {
  isDM?: boolean;
};

const emptyDismissalCounts = (): NotificationDismissalCounts => ({
  total: 0,
  byRoom: {}
});

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
    default:
      return {
        isDM: false,
        spaceName: null,
        roomId: null,
        roomName: null,
        eventId: null,
        threadRootId: null
      };
  }
}

/**
 * Notification state store.
 * Manages notifications for the current user with real-time sync.
 */
export class NotificationStore {
  #client: Client;
  #locallyDismissedNotificationIds = new SvelteSet<string>();
  notifications = $state<NotificationItem[]>([]);
  /**
   * Server display name, captured alongside the notification list and used
   * by getLocationString() for non-DM notifications. Post-#330 PR(a) the
   * notification's space name no longer comes from the per-notification
   * fragment — it's the instance name.
   */
  serverName = $state<string | null>(null);
  unreadNotificationCount = $state(0);
  loading = $state(false);
  error = $state<string | null>(null);

  constructor(client: Client) {
    this.#client = client;
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

  getCachedRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): NotificationItem | undefined {
    return options.isDM ? this.getDMRoomNotification(roomId) : this.getRoomNotification(roomId);
  }

  /**
   * Dismiss all thread-scoped notifications (replies + mentions) for a thread.
   * Called when a user opens a thread to clear the notification indicator.
   */
  async dismissThreadNotifications(threadRootId: string): Promise<NotificationDismissalCounts> {
    const threadNotifications = this.notifications.filter(
      (n) =>
        (n.__typename === 'ReplyNotificationItem' && n.replyInThread === threadRootId) ||
        (n.__typename === 'MentionNotificationItem' && n.mentionInThread === threadRootId)
    );

    return this.#dismissNotifications(threadNotifications);
  }

  /**
   * Dismiss room-level mention notifications for a specific room.
   * Called when a user enters a room. Thread-scoped mentions are NOT dismissed
   * here — they're dismissed when the user opens the specific thread (via
   * dismissThreadNotifications), matching the symmetry with reply notifications.
   */
  async dismissMentionNotifications(roomId: string): Promise<NotificationDismissalCounts> {
    const mentionNotifications = this.notifications.filter(
      (n) =>
        n.__typename === 'MentionNotificationItem' &&
        !n.mentionInThread &&
        n.mentionRoom?.id === roomId
    );

    return this.#dismissNotifications(mentionNotifications);
  }

  /**
   * Dismiss room-level reply notifications for a specific room.
   * Called when a user enters a room to clear reply notification indicators.
   * Only dismisses room-level replies (not thread replies, which are dismissed
   * separately when opening the specific thread via dismissThreadNotifications).
   */
  async dismissRoomReplyNotifications(roomId: string): Promise<NotificationDismissalCounts> {
    const roomReplyNotifications = this.notifications.filter(
      (n) =>
        n.__typename === 'ReplyNotificationItem' && !n.replyInThread && n.replyRoom?.id === roomId
    );

    return this.#dismissNotifications(roomReplyNotifications);
  }

  /**
   * Dismiss all room message notifications for a specific room.
   * Called when a user enters a room to clear "all messages" notification indicators.
   */
  async dismissRoomMessageNotifications(roomId: string): Promise<NotificationDismissalCounts> {
    const roomMsgNotifications = this.notifications.filter(
      (n) => n.__typename === 'RoomMessageNotificationItem' && n.roomMsgRoom?.id === roomId
    );

    return this.#dismissNotifications(roomMsgNotifications);
  }

  /**
   * Dismiss all DM notifications for a specific conversation.
   * Called when a user enters a DM conversation to clear notification indicators.
   */
  async dismissDMNotifications(roomId: string): Promise<NotificationDismissalCounts> {
    const dmNotifications = this.notifications.filter(
      (n) => n.__typename === 'DMMessageNotificationItem' && n.room.id === roomId
    );

    return this.#dismissNotifications(dmNotifications);
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
      const result = await this.#client.query(NotificationsQueryDoc, {}).toPromise();

      if (result.error) {
        this.error = result.error.message;
        console.error('Failed to fetch notifications:', result.error);
        return;
      }

      if (result.data?.viewer) {
        this.notifications = useFragment(
          NotificationItemViewFragmentDoc,
          result.data.viewer.notifications.items
        );
        this.unreadNotificationCount = result.data.viewer.notifications.totalCount;
      }
      // Capture the instance display name lazily — used by getLocationString
      // for non-DM notifications. Failure here is non-fatal; the UI just
      // omits the "in <name>" suffix.
      try {
        const nameRes = await this.#client
          .query(InstanceNameQueryDoc, {}, { requestPolicy: 'cache-first' })
          .toPromise();
        this.serverName = nameRes.data?.server?.profile.name ?? null;
      } catch {
        // ignore
      }
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
   * Room sidebar badges are sourced from Room.viewerNotifications.totalCount,
   * so badge clicks need the same scoped source when the global cached page is
   * empty, stale, or does not include this room's notification.
   */
  async fetchRoomNotification(roomId: string): Promise<RoomNotificationLookup> {
    try {
      const result = await this.#client.query(RoomNotificationQueryDoc, { roomId }).toPromise();

      if (result.error) {
        this.error = result.error.message;
        console.error('Failed to fetch room notification:', result.error);
        return { ok: false, totalCount: null, notification: null };
      }

      const connection = result.data?.room?.viewerNotifications;
      if (!connection) {
        return { ok: true, totalCount: null, notification: null };
      }

      const notification = connection.items[0]
        ? useFragment(NotificationItemViewFragmentDoc, connection.items[0])
        : null;
      if (notification) {
        this.#upsertNotification(notification);
      }

      return {
        ok: true,
        totalCount: connection.totalCount,
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
      const result = await this.#client.query(HasNotificationsQueryDoc, {}).toPromise();
      return result.data?.viewer?.hasNotifications ?? false;
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
      const result = await this.#client
        .mutation(DismissNotificationMutationDoc, { input: { notificationId } })
        .toPromise();

      if (result.error || !result.data?.dismissNotification) {
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
      const result = await this.#client
        .mutation(DismissAllNotificationsMutationDoc, {})
        .toPromise();

      if (result.error || result.data?.dismissAllNotifications == null) {
        for (const notification of original) {
          this.#locallyDismissedNotificationIds.delete(notification.id);
        }
        this.notifications = original;
        this.unreadNotificationCount = originalCount;
        return 0;
      }
      return result.data.dismissAllNotifications;
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

  async #dismissNotifications(
    notifications: NotificationItem[]
  ): Promise<NotificationDismissalCounts> {
    if (notifications.length === 0) return emptyDismissalCounts();

    const results = await Promise.all(
      notifications.map(async (notification) => ({
        target: notificationTarget(notification),
        dismissed: await this.dismiss(notification.id)
      }))
    );

    const counts = emptyDismissalCounts();
    for (const result of results) {
      if (!result.dismissed) continue;
      counts.total += 1;
      const roomId = result.target.roomId;
      if (roomId) counts.byRoom[roomId] = (counts.byRoom[roomId] ?? 0) + 1;
    }
    return counts;
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
   * The "in <name>" suffix uses the instance display name (server = instance
   * post-#330 PR(a)), captured alongside the notification list.
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
