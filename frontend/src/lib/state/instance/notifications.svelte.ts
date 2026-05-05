import { SvelteSet } from 'svelte/reactivity';
import { graphql } from '$lib/gql';
import type { NotificationsQuery } from '$lib/gql/graphql';
import type { Client } from '@urql/svelte';
import { resolve } from '$app/paths';
import { instanceIdToSegment } from '$lib/navigation';

// GraphQL queries and mutations
const NotificationsQueryDoc = graphql(`
  query Notifications {
    notifications {
      __typename
      ... on DMMessageNotificationItem {
        id
        createdAt
        actor {
          id
          login
          displayName
          avatarUrl(width: 96, height: 96)
          presenceStatus
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
          id
          login
          displayName
          avatarUrl(width: 96, height: 96)
          presenceStatus
        }
        summary
        mentionSpace: space {
          id
          name
        }
        mentionRoom: room {
          id
          name
        }
        mentionEventId: eventId
        mentionInThread: inThread
      }
      ... on ReplyNotificationItem {
        id
        createdAt
        actor {
          id
          login
          displayName
          avatarUrl(width: 96, height: 96)
          presenceStatus
        }
        summary
        replySpace: space {
          id
          name
        }
        replyRoom: room {
          id
          name
        }
        replyEventId: eventId
        inReplyToId
        replyInThread: inThread
      }
      ... on RoomMessageNotificationItem {
        id
        createdAt
        actor {
          id
          login
          displayName
          avatarUrl(width: 96, height: 96)
          presenceStatus
        }
        summary
        roomMsgSpace: space {
          id
          name
        }
        roomMsgRoom: room {
          id
          name
        }
        roomMsgEventId: eventId
      }
    }
  }
`);

const HasNotificationsQueryDoc = graphql(`
  query HasNotifications {
    hasNotifications
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
export type NotificationItem = NotificationsQuery['notifications'][number];

/**
 * Normalized view of a notification's target (where it points to in the app).
 * Avoids `__typename` switches at every read site — see {@link notificationTarget}.
 */
export type NotificationTarget = {
  isDM: boolean;
  spaceId: string | null;
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
        spaceId: null,
        spaceName: null,
        roomId: n.room.id,
        roomName: null,
        eventId: null,
        threadRootId: null
      };
    case 'MentionNotificationItem':
      return {
        isDM: false,
        spaceId: n.mentionSpace?.id ?? null,
        spaceName: n.mentionSpace?.name ?? null,
        roomId: n.mentionRoom?.id ?? null,
        roomName: n.mentionRoom?.name ?? null,
        eventId: n.mentionEventId ?? null,
        threadRootId: n.mentionInThread ?? null
      };
    case 'ReplyNotificationItem':
      return {
        isDM: false,
        spaceId: n.replySpace?.id ?? null,
        spaceName: n.replySpace?.name ?? null,
        roomId: n.replyRoom?.id ?? null,
        roomName: n.replyRoom?.name ?? null,
        eventId: n.replyEventId ?? null,
        threadRootId: n.replyInThread ?? null
      };
    case 'RoomMessageNotificationItem':
      return {
        isDM: false,
        spaceId: n.roomMsgSpace?.id ?? null,
        spaceName: n.roomMsgSpace?.name ?? null,
        roomId: n.roomMsgRoom?.id ?? null,
        roomName: n.roomMsgRoom?.name ?? null,
        eventId: n.roomMsgEventId ?? null,
        threadRootId: null
      };
    default:
      return {
        isDM: false,
        spaceId: null,
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
  notifications = $state<NotificationItem[]>([]);
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
   * Check if a specific space has pending notifications.
   */
  hasSpaceNotification(spaceId: string): boolean {
    return this.notifications.some((n) => notificationTarget(n).spaceId === spaceId);
  }

  /**
   * Get the most recent notification for a space.
   * Notifications are sorted most-recent-first, so .find returns the freshest.
   */
  getSpaceNotification(spaceId: string): NotificationItem | undefined {
    return this.notifications.find((n) => notificationTarget(n).spaceId === spaceId);
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

    // Dismiss each one (in parallel)
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

    // Dismiss each one (in parallel)
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

    // Dismiss each one (in parallel)
    await Promise.all(dmNotifications.map((n) => this.dismiss(n.id)));
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

      if (result.data) {
        this.notifications = result.data.notifications;
      }
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
      const result = await this.#client.query(HasNotificationsQueryDoc, {}).toPromise();
      return result.data?.hasNotifications ?? false;
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
      const result = await this.#client
        .mutation(DismissNotificationMutationDoc, { input: { notificationId } })
        .toPromise();

      if (result.error || !result.data?.dismissNotification) {
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
      const result = await this.#client
        .mutation(DismissAllNotificationsMutationDoc, {})
        .toPromise();

      if (result.error || result.data?.dismissAllNotifications == null) {
        this.notifications = original;
        return 0;
      }
      return result.data.dismissAllNotifications;
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
    // Refetch to get the new notification with full data
    await this.fetch();
  }

  /**
   * Remove a notification by ID (for cross-device sync).
   */
  removeNotification(notificationId: string) {
    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
  }

  /**
   * Get location string for a notification (e.g., "#general in My Space").
   * Returns null for DM notifications and any notification missing names.
   */
  getLocationString(notification: NotificationItem): string | null {
    const t = notificationTarget(notification);
    if (t.isDM || !t.spaceName || !t.roomName) return null;
    return `#${t.roomName} in ${t.spaceName}`;
  }

  /**
   * Build a clean (no `?highlight=`) destination path for a notification.
   * Use this with `PendingHighlightStore.set()` to deliver the highlight
   * intent without polluting the URL.
   */
  getCleanPath(instanceId: string, notification: NotificationItem): string {
    const seg = instanceIdToSegment(instanceId);
    const t = notificationTarget(notification);

    if (t.isDM && t.roomId) {
      // DMs are now rooms on the Server (#330 phase 3) — use the standard
      // room URL rather than the legacy /chat/dm/... path.
      return resolve('/chat/[instanceId]/(chrome)/[roomId]', {
        instanceId: seg,
        roomId: t.roomId
      });
    }
    if (!t.spaceId || !t.roomId) {
      return resolve('/chat/[instanceId]', { instanceId: seg });
    }
    if (t.threadRootId) {
      return resolve('/chat/[instanceId]/(chrome)/[roomId]/[threadId]', {
        instanceId: seg,
        roomId: t.roomId,
        threadId: t.threadRootId
      });
    }
    return resolve('/chat/[instanceId]/(chrome)/[roomId]', {
      instanceId: seg,
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
  getNavigationPath(instanceId: string, notification: NotificationItem): string {
    const seg = instanceIdToSegment(instanceId);
    const t = notificationTarget(notification);

    if (t.isDM && t.roomId) {
      // DMs are now rooms on the Server (#330 phase 3) — use the standard
      // room URL rather than the legacy /chat/dm/... path.
      return resolve('/chat/[instanceId]/(chrome)/[roomId]', {
        instanceId: seg,
        roomId: t.roomId
      });
    }

    if (!t.spaceId || !t.roomId) {
      return resolve('/chat/[instanceId]', { instanceId: seg });
    }

    if (t.threadRootId && t.eventId) {
      return (
        resolve('/chat/[instanceId]/(chrome)/[roomId]/[threadId]', {
          instanceId: seg,
          roomId: t.roomId,
          threadId: t.threadRootId
        }) +
        '?highlight=' +
        t.eventId
      );
    }

    const roomPath = resolve('/chat/[instanceId]/(chrome)/[roomId]', {
      instanceId: seg,
      roomId: t.roomId
    });
    return t.eventId ? `${roomPath}?highlight=${t.eventId}` : roomPath;
  }
}
