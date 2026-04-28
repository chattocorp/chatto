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
   * Check if a specific room has pending notifications (thread replies or mentions).
   */
  hasRoomNotification(roomId: string): boolean {
    return this.notifications.some((n) => {
      if (n.__typename === 'ReplyNotificationItem') {
        return n.replyRoom?.id === roomId;
      }
      if (n.__typename === 'MentionNotificationItem') {
        return n.mentionRoom?.id === roomId;
      }
      if (n.__typename === 'RoomMessageNotificationItem') {
        return n.roomMsgRoom?.id === roomId;
      }
      return false;
    });
  }

  /**
   * Check if a specific space has pending notifications (thread replies or mentions).
   */
  hasSpaceNotification(spaceId: string): boolean {
    return this.notifications.some((n) => {
      if (n.__typename === 'ReplyNotificationItem') {
        return n.replySpace?.id === spaceId;
      }
      if (n.__typename === 'MentionNotificationItem') {
        return n.mentionSpace?.id === spaceId;
      }
      if (n.__typename === 'RoomMessageNotificationItem') {
        return n.roomMsgSpace?.id === spaceId;
      }
      return false;
    });
  }

  /**
   * Get the most recent notification for a space.
   * Returns undefined if no notifications exist for the space.
   * Uses .find() which returns the first match - notifications are sorted by most recent first.
   */
  getSpaceNotification(spaceId: string): NotificationItem | undefined {
    return this.notifications.find((n) => {
      if (n.__typename === 'ReplyNotificationItem') {
        return n.replySpace?.id === spaceId;
      }
      if (n.__typename === 'MentionNotificationItem') {
        return n.mentionSpace?.id === spaceId;
      }
      if (n.__typename === 'RoomMessageNotificationItem') {
        return n.roomMsgSpace?.id === spaceId;
      }
      return false;
    });
  }

  /**
   * Get the most recent notification for a room.
   * Returns undefined if no notifications exist for the room.
   * Uses .find() which returns the first match - notifications are sorted by most recent first.
   */
  getRoomNotification(roomId: string): NotificationItem | undefined {
    return this.notifications.find((n) => {
      if (n.__typename === 'ReplyNotificationItem') {
        return n.replyRoom?.id === roomId;
      }
      if (n.__typename === 'MentionNotificationItem') {
        return n.mentionRoom?.id === roomId;
      }
      if (n.__typename === 'RoomMessageNotificationItem') {
        return n.roomMsgRoom?.id === roomId;
      }
      return false;
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
   * Dismiss all reply notifications for a specific thread.
   * Called when a user opens a thread to clear the notification indicator.
   */
  async dismissThreadNotifications(threadRootId: string): Promise<void> {
    // Find all thread reply notifications for this thread (not room-level replies)
    const threadNotifications = this.notifications.filter(
      (n) => n.__typename === 'ReplyNotificationItem' && n.replyInThread === threadRootId
    );

    // Dismiss each one (in parallel)
    await Promise.all(threadNotifications.map((n) => this.dismiss(n.id)));
  }

  /**
   * Dismiss mention notifications for a specific room.
   * Called when a user enters a room to clear mention notification indicators.
   * Note: Thread reply notifications are NOT dismissed here - they should only
   * be dismissed when the user opens the specific thread (via dismissThreadNotifications).
   */
  async dismissMentionNotifications(roomId: string): Promise<void> {
    const mentionNotifications = this.notifications.filter(
      (n) => n.__typename === 'MentionNotificationItem' && n.mentionRoom?.id === roomId
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
   */
  async fetch() {
    this.loading = true;
    this.error = null;

    try {
      const result = await this.#client.query(NotificationsQueryDoc, {}).toPromise();

      if (result.error) {
        throw new Error(result.error.message);
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
   * Dismiss a single notification.
   */
  async dismiss(notificationId: string): Promise<boolean> {
    try {
      const result = await this.#client
        .mutation(DismissNotificationMutationDoc, { input: { notificationId } })
        .toPromise();

      if (result.data?.dismissNotification) {
        // Remove from local state immediately for snappy UI
        this.notifications = this.notifications.filter((n) => n.id !== notificationId);
        return true;
      }
      return false;
    } catch (e) {
      console.error('Failed to dismiss notification:', e);
      return false;
    }
  }

  /**
   * Dismiss all notifications.
   */
  async dismissAll(): Promise<number> {
    try {
      const result = await this.#client
        .mutation(DismissAllNotificationsMutationDoc, {})
        .toPromise();

      const count = result.data?.dismissAllNotifications ?? 0;
      if (count > 0) {
        // Clear local state
        this.notifications = [];
      }
      return count;
    } catch (e) {
      console.error('Failed to dismiss all notifications:', e);
      return 0;
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
    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
  }

  /**
   * Get location string for a notification (e.g., "#general in My Space").
   * Returns null for DM notifications (no space/room context needed).
   */
  getLocationString(notification: NotificationItem): string | null {
    switch (notification.__typename) {
      case 'MentionNotificationItem': {
        const spaceName = notification.mentionSpace?.name;
        const roomName = notification.mentionRoom?.name;
        if (spaceName && roomName) {
          return `#${roomName} in ${spaceName}`;
        }
        return null;
      }

      case 'ReplyNotificationItem': {
        const spaceName = notification.replySpace?.name;
        const roomName = notification.replyRoom?.name;
        if (spaceName && roomName) {
          return `#${roomName} in ${spaceName}`;
        }
        return null;
      }

      case 'RoomMessageNotificationItem': {
        const spaceName = notification.roomMsgSpace?.name;
        const roomName = notification.roomMsgRoom?.name;
        if (spaceName && roomName) {
          return `#${roomName} in ${spaceName}`;
        }
        return null;
      }

      default:
        return null;
    }
  }

  /**
   * Get navigation info for a notification.
   * Returns the path to navigate to when acting on the notification.
   */
  getNavigationPath(instanceId: string, notification: NotificationItem): string {
    const seg = instanceIdToSegment(instanceId);

    switch (notification.__typename) {
      case 'DMMessageNotificationItem':
        return resolve('/chat/dm/[instanceSegment]/[conversationId]', {
          instanceSegment: seg,
          conversationId: notification.room.id
        });

      case 'MentionNotificationItem': {
        // Navigate to the room, optionally with eventId to scroll to message
        // Using aliased fields from query
        const spaceId = notification.mentionSpace?.id;
        const roomId = notification.mentionRoom?.id;
        const eventId = notification.mentionEventId;
        if (eventId && spaceId && roomId) {
          return (
            resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
              instanceId: seg,
              spaceId,
              roomId
            }) +
            '?highlight=' +
            eventId
          );
        }
        return spaceId && roomId
          ? resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
              instanceId: seg,
              spaceId,
              roomId
            })
          : resolve('/chat/[instanceId]', { instanceId: seg });
      }

      case 'ReplyNotificationItem': {
        // Using aliased fields from query
        const spaceId = notification.replySpace?.id;
        const roomId = notification.replyRoom?.id;
        const eventId = notification.replyEventId;
        const threadRootId = notification.replyInThread;
        if (threadRootId && spaceId && roomId && eventId) {
          // Thread reply: navigate to the thread (using thread root) and highlight the new reply
          return (
            resolve('/chat/[instanceId]/[spaceId]/[roomId]/[threadId]', {
              instanceId: seg,
              spaceId,
              roomId,
              threadId: threadRootId
            }) +
            '?highlight=' +
            eventId
          );
        }
        // Room-level reply: navigate to room and highlight the reply message
        if (eventId && spaceId && roomId) {
          return (
            resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
              instanceId: seg,
              spaceId,
              roomId
            }) +
            '?highlight=' +
            eventId
          );
        }
        return spaceId && roomId
          ? resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
              instanceId: seg,
              spaceId,
              roomId
            })
          : resolve('/chat/[instanceId]', { instanceId: seg });
      }

      case 'RoomMessageNotificationItem': {
        const spaceId = notification.roomMsgSpace?.id;
        const roomId = notification.roomMsgRoom?.id;
        const eventId = notification.roomMsgEventId;
        if (eventId && spaceId && roomId) {
          return (
            resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
              instanceId: seg,
              spaceId,
              roomId
            }) +
            '?highlight=' +
            eventId
          );
        }
        return spaceId && roomId
          ? resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
              instanceId: seg,
              spaceId,
              roomId
            })
          : resolve('/chat/[instanceId]', { instanceId: seg });
      }

      default:
        return resolve('/chat/[instanceId]', { instanceId: seg });
    }
  }
}
