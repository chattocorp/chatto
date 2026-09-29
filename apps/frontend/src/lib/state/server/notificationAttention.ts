/**
 * The notifications that need the user's attention on one server.
 *
 * The client's `NotificationStore` keeps the server's occurrences and counts.
 * An occurrence for content that the user looks at in a focused window does
 * not need attention: this view leaves it out of badges, indicators, and
 * sounds, without changing its server read state. See `ReadViewRegistry`.
 */

import { computed } from '@chatto/client/reactivity';
import {
  NotificationAttentionLevel,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import {
  isDMNotification,
  matchesRoomNotification,
  notificationTarget,
  type NotificationStore,
  type RoomNotificationLookup,
  type RoomNotificationResolveOptions
} from '@chatto/client/server/notifications';
import type { ReadViewRegistry } from './readViews';

/** Badge counts after viewed occurrences are left out. */
export type AttentionCounts = {
  unreadNotificationCount: number;
  importantUnreadNotificationCount: number;
  roomUnreadCounts: Record<string, number>;
  roomImportantUnreadCounts: Record<string, number>;
};

/** Notification attention of one server; see the module documentation. */
export class NotificationAttention {
  readonly #notifications: NotificationStore;
  readonly #readViews: ReadViewRegistry;

  readonly #occurrences = computed(() =>
    this.#notifications.unreadOccurrences.filter((row) => this.needsAttention(row))
  );

  readonly #counts = computed((): AttentionCounts => {
    const notifications = this.#notifications;
    const suppressed = notifications.occurrences.filter(
      (row) => row.unread && !this.needsAttention(row)
    );
    const roomUnreadCounts = { ...notifications.roomUnreadCounts };
    const roomImportantUnreadCounts = { ...notifications.roomImportantUnreadCounts };
    let importantCount = 0;
    for (const row of suppressed) {
      const important = row.attentionLevel === NotificationAttentionLevel.IMPORTANT;
      if (important) importantCount++;
      if (row.room) {
        roomUnreadCounts[row.room.id] = Math.max(0, (roomUnreadCounts[row.room.id] ?? 0) - 1);
        if (important)
          roomImportantUnreadCounts[row.room.id] = Math.max(
            0,
            (roomImportantUnreadCounts[row.room.id] ?? 0) - 1
          );
      }
    }
    return {
      unreadNotificationCount: Math.max(
        0,
        notifications.unreadNotificationCount - suppressed.length
      ),
      importantUnreadNotificationCount: Math.max(
        0,
        notifications.importantUnreadNotificationCount - importantCount
      ),
      roomUnreadCounts,
      roomImportantUnreadCounts
    };
  });

  constructor(notifications: NotificationStore, readViews: ReadViewRegistry) {
    this.#notifications = notifications;
    this.#readViews = readViews;
  }

  /** Whether an occurrence needs attention: unread and not viewed. Also decides in-app sounds. */
  needsAttention(occurrence: NotificationOccurrenceItem): boolean {
    const target = notificationTarget(occurrence);
    return occurrence.unread && !this.#readViews.covers(target.roomId, target.threadRootId);
  }

  /** Loaded unread occurrences that need attention, newest first. Reactive. */
  get occurrences(): NotificationOccurrenceItem[] {
    return this.#occurrences.get();
  }

  /** Badge counts without viewed occurrences. Reactive. */
  get counts(): AttentionCounts {
    return this.#counts.get();
  }

  /** Whether a thread has an occurrence that needs attention. */
  hasThreadNotification(threadRootId: string): boolean {
    return this.occurrences.some((n) => notificationTarget(n).threadRootId === threadRootId);
  }

  /** Whether a room outside direct messages has an occurrence that needs attention. */
  hasRoomNotification(roomId: string): boolean {
    return this.occurrences.some((n) => {
      const target = notificationTarget(n);
      return !target.isDM && target.roomId === roomId;
    });
  }

  /** Whether an occurrence outside direct messages needs attention. */
  hasNonDMNotifications(): boolean {
    return this.occurrences.some((n) => !isDMNotification(n));
  }

  /** The newest occurrence outside direct messages with a supported target. */
  getNonDMNotification(): NotificationOccurrenceItem | undefined {
    return this.occurrences.find((n) => n.targetSupported && !isDMNotification(n));
  }

  /** Whether a direct message has an occurrence that needs attention. */
  hasDMNotifications(): boolean {
    return this.occurrences.some((n) => isDMNotification(n));
  }

  /** The newest direct-message occurrence that needs attention. */
  getDMNotification(): NotificationOccurrenceItem | undefined {
    return this.occurrences.find((n) => isDMNotification(n));
  }

  /** Whether one direct message has an occurrence that needs attention. */
  hasDMRoomNotification(roomId: string): boolean {
    return this.occurrences.some((n) => isDMNotification(n) && n.room?.id === roomId);
  }

  /** The newest loaded occurrence of a room that needs attention. */
  getCachedRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): NotificationOccurrenceItem | undefined {
    return this.occurrences.find((n) => matchesRoomNotification(n, roomId, options));
  }

  /**
   * The newest occurrence of a room: a loaded one that needs attention, or
   * else the server's.
   */
  resolveRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): Promise<RoomNotificationLookup> {
    const cached = this.getCachedRoomNotification(roomId, options);
    if (cached) return Promise.resolve({ ok: true, totalCount: null, notification: cached });
    return this.#notifications.fetchRoomNotification(roomId, options);
  }
}
