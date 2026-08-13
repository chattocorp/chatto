import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import {
  NotificationItemKind,
  NotificationAttentionLevel,
  NotificationDeliveryIntensity,
  occurrenceAsNotificationItem,
  type DirectMessageNotificationItem,
  type MentionNotificationItem,
  type NotificationAPI,
  type NotificationOccurrenceItem,
  type NotificationOccurrencePage,
  type NotificationPolicyItem,
  type NotificationReason,
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
      roomId: n.room.id,
      roomName: null,
      eventId: n.eventId ?? null,
      threadRootId: null
    };
  }
  if (isMentionNotification(n)) {
    return {
      isDM: false,
      roomId: n.mentionRoom?.id ?? null,
      roomName: n.mentionRoom?.name ?? null,
      eventId: n.mentionEventId ?? null,
      threadRootId: n.mentionInThread ?? null
    };
  }
  if (isReplyNotification(n)) {
    return {
      isDM: false,
      roomId: n.replyRoom?.id ?? null,
      roomName: n.replyRoom?.name ?? null,
      eventId: n.replyEventId ?? null,
      threadRootId: n.replyInThread ?? null
    };
  }
  if (isRoomMessageNotification(n)) {
    return {
      isDM: false,
      roomId: n.roomMsgRoom?.id ?? null,
      roomName: n.roomMsgRoom?.name ?? null,
      eventId: n.roomMsgEventId ?? null,
      threadRootId: n.roomMsgThreadRootId ?? null
    };
  }
  return {
    isDM: false,
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
  #fetchGeneration = 0;
  // Authoritative replacements suppress rollback of older optimistic state.
  #authoritativeGeneration = 0;
  // Exact deletions are independent; delete-all supersedes all of them.
  #deletionSequence = 0;
  #deleteAllGeneration = 0;
  #pendingDeletionById = new SvelteMap<string, number>();
  notifications = $state<NotificationItem[]>([]);
  occurrences = $state.raw<NotificationOccurrenceItem[]>([]);
  unreadNotificationCount = $state(0);
  importantUnreadNotificationCount = $state(0);
  nextExpiryAt = $state<string | null>(null);
  /** Advances only for realtime invalidations, including changes made in another session. */
  viewInvalidationVersion = $state(0);
  loading = $state(false);
  hasLoaded = $state(false);
  error = $state<string | null>(null);

  constructor(api: NotificationAPI) {
    this.#api = api;
  }

  get count() {
    return this.notifications.length;
  }

  setUnreadNotificationCount(count: number, importantCount = count): void {
    this.unreadNotificationCount = Math.max(0, count);
    this.importantUnreadNotificationCount = Math.max(0, Math.min(importantCount, count));
  }

  /** Replace notification state from the realtime projection. */
  replaceOccurrenceProjection(page: NotificationOccurrencePage): void {
    this.#fetchGeneration++;
    this.#authoritativeGeneration++;
    this.#pendingDeletionById.clear();
    this.occurrences = page.occurrences;
    this.notifications = page.occurrences
      .filter((occurrence) => occurrence.unread)
      .map(occurrenceAsNotificationItem)
      .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
      .slice(0, 50);
    this.unreadNotificationCount = page.unreadCount;
    this.importantUnreadNotificationCount = Math.max(
      0,
      Math.min(page.importantUnreadCount, page.unreadCount)
    );
    this.nextExpiryAt = page.nextExpiryAt ?? null;
    this.loading = false;
    this.hasLoaded = true;
    this.error = null;
  }

  invalidateViews(): void {
    this.viewInvalidationVersion++;
  }

  /** Invalidate projection-owned state while a compacted reset hydrates. */
  resetProjectionState(): void {
    this.#fetchGeneration++;
    this.#authoritativeGeneration++;
    this.#pendingDeletionById.clear();
    this.notifications = [];
    this.occurrences = [];
    this.unreadNotificationCount = 0;
    this.importantUnreadNotificationCount = 0;
    this.nextExpiryAt = null;
    this.loading = true;
    // The empty reset boundary is already authoritative. Keep this true so
    // badge synchronisation clears stale native notification counts even when
    // a later snapshot frame never arrives.
    this.hasLoaded = true;
    this.error = null;
  }

  /** Remove copied profile data for an account deleted from the projection. */
  scrubUser(userId: string): void {
    let changed = false;
    const notifications = this.notifications.map((notification) => {
      if (notification.actor?.id !== userId) return notification;
      changed = true;
      return {
        ...notification,
        actor: null,
        summary: redactedNotificationSummary(notification.kind)
      };
    });
    if (changed) this.notifications = notifications;

    const occurrences = this.occurrences.map((occurrence) =>
      occurrence.actor?.id === userId ? { ...occurrence, actor: null } : occurrence
    );
    if (occurrences.some((occurrence, index) => occurrence !== this.occurrences[index])) {
      this.occurrences = occurrences;
    }
  }

  /** Drop notification payloads for a room at an authorization boundary. */
  clearRoom(roomId: string): void {
    this.#authoritativeGeneration++;
    this.#pendingDeletionById.clear();
    const removedUnreadOccurrences = this.occurrences.filter(
      (occurrence) => occurrence.unread && occurrence.room?.id === roomId
    ).length;
    const removedImportantUnreadOccurrences = this.occurrences.filter(
      (occurrence) =>
        occurrence.unread &&
        occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT &&
        occurrence.room?.id === roomId
    ).length;
    this.occurrences = this.occurrences.filter((occurrence) => occurrence.room?.id !== roomId);

    const notifications = this.notifications.filter(
      (notification) => notificationTarget(notification).roomId !== roomId
    );
    const removed = this.notifications.length - notifications.length;
    if (removed > 0) this.notifications = notifications;
    if (removedUnreadOccurrences > 0) {
      this.unreadNotificationCount = Math.max(
        0,
        this.unreadNotificationCount - removedUnreadOccurrences
      );
    }
    if (removedImportantUnreadOccurrences > 0) {
      this.importantUnreadNotificationCount = Math.max(
        0,
        this.importantUnreadNotificationCount - removedImportantUnreadOccurrences
      );
    }
  }

  /**
   * Get the set of thread root IDs that have pending reply notifications.
   * Used to show notification indicators on thread buttons.
   */
  get threadsWithNotifications(): SvelteSet<string> {
    const threadIds = new SvelteSet<string>();
    for (const n of this.notifications) {
      const threadRootId = notificationTarget(n).threadRootId;
      if (threadRootId) threadIds.add(threadRootId);
    }
    return threadIds;
  }

  /**
   * Check if a specific thread has unread notification occurrences.
   */
  hasThreadNotification(threadRootId: string): boolean {
    return this.notifications.some((n) => notificationTarget(n).threadRootId === threadRootId);
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

  /** Check if the server has any pending non-DM notifications. */
  hasNonDMNotifications(): boolean {
    return this.notifications.some((n) => !notificationTarget(n).isDM);
  }

  /**
   * Get the most recent non-DM notification.
   * Notifications are sorted most-recent-first, so .find returns the freshest.
   */
  getNonDMNotification(): NotificationItem | undefined {
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
   * Check if a specific DM conversation has unread notification occurrences.
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
    const generation = ++this.#fetchGeneration;
    this.loading = true;
    this.error = null;

    try {
      const page = await this.#api.listNotificationOccurrences(50);
      if (generation !== this.#fetchGeneration) return;

      this.replaceOccurrenceProjection(page);
    } catch (e) {
      if (generation !== this.#fetchGeneration) return;
      this.error = e instanceof Error ? e.message : 'Failed to fetch notifications';
      console.error('Failed to fetch notifications:', e);
    } finally {
      if (generation === this.#fetchGeneration) {
        this.loading = false;
      }
    }
  }

  async fetchPage(offset = 0): Promise<NotificationOccurrencePage> {
    const page = await this.#api.listNotificationOccurrences(50, offset);
    if (offset === 0) this.replaceOccurrenceProjection(page);
    return page;
  }

  async markOccurrenceRead(notificationId: string): Promise<void> {
    await this.#api.markNotificationRead(notificationId);
  }

  /** Delete exact occurrences optimistically, restoring them on failure. */
  async deleteOccurrences(
    notificationIds: string[],
    knownCounts?: { unread: number; importantUnread: number }
  ): Promise<void> {
    const uniqueIds = [...new SvelteSet(notificationIds)];
    const removedIds = new SvelteSet(uniqueIds);
    const removedOccurrences = this.occurrences.filter((occurrence) =>
      removedIds.has(occurrence.id)
    );
    const removedNotifications = this.notifications.filter((notification) =>
      removedIds.has(notification.id)
    );
    const authoritativeGeneration = this.#authoritativeGeneration;
    const deleteAllGeneration = this.#deleteAllGeneration;
    const mutation = ++this.#deletionSequence;
    for (const id of uniqueIds) this.#pendingDeletionById.set(id, mutation);
    // Supersede stale list work without starting a second list request. The
    // worker's realtime replacement performs authoritative reconciliation.
    this.#fetchGeneration++;
    this.loading = false;
    this.occurrences = this.occurrences.filter((occurrence) => !removedIds.has(occurrence.id));
    this.notifications = this.notifications.filter(
      (notification) => !removedIds.has(notification.id)
    );
    const removedUnreadCount =
      knownCounts?.unread ?? removedOccurrences.filter((occurrence) => occurrence.unread).length;
    const removedImportantUnreadCount =
      knownCounts?.importantUnread ??
      removedOccurrences.filter(
        (occurrence) =>
          occurrence.unread &&
          occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT
      ).length;
    this.unreadNotificationCount = Math.max(0, this.unreadNotificationCount - removedUnreadCount);
    this.importantUnreadNotificationCount = Math.max(
      0,
      this.importantUnreadNotificationCount - removedImportantUnreadCount
    );
    this.nextExpiryAt = earliestNotificationOccurrenceExpiry(this.occurrences);

    try {
      await this.#api.batchDeleteNotificationOccurrences(uniqueIds);
    } catch (error) {
      const rollbackIds = new SvelteSet(
        uniqueIds.filter((id) => this.#pendingDeletionById.get(id) === mutation)
      );
      if (
        this.#authoritativeGeneration === authoritativeGeneration &&
        this.#deleteAllGeneration === deleteAllGeneration &&
        rollbackIds.size > 0
      ) {
        this.occurrences = mergeNotificationOccurrences(
          this.occurrences,
          removedOccurrences.filter((occurrence) => rollbackIds.has(occurrence.id))
        );
        this.notifications = mergeNotificationItems(
          this.notifications,
          removedNotifications.filter((notification) => rollbackIds.has(notification.id))
        );
        const rollbackUnreadCount =
          knownCounts !== undefined && rollbackIds.size === removedIds.size
            ? knownCounts.unread
            : removedOccurrences.filter(
                (occurrence) => rollbackIds.has(occurrence.id) && occurrence.unread
              ).length;
        const rollbackImportantUnreadCount =
          knownCounts !== undefined && rollbackIds.size === removedIds.size
            ? knownCounts.importantUnread
            : removedOccurrences.filter(
                (occurrence) =>
                  rollbackIds.has(occurrence.id) &&
                  occurrence.unread &&
                  occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT
              ).length;
        this.unreadNotificationCount += rollbackUnreadCount;
        this.importantUnreadNotificationCount += rollbackImportantUnreadCount;
        this.nextExpiryAt = earliestNotificationOccurrenceExpiry(this.occurrences);
      }
      throw error;
    } finally {
      for (const id of uniqueIds) {
        if (this.#pendingDeletionById.get(id) === mutation) {
          this.#pendingDeletionById.delete(id);
        }
      }
    }
  }

  /** Delete every current occurrence optimistically for this server. */
  async deleteAllOccurrences(): Promise<void> {
    const originalOccurrences = this.occurrences;
    const originalNotifications = this.notifications;
    const originalCount = this.unreadNotificationCount;
    const originalImportantCount = this.importantUnreadNotificationCount;
    const originalNextExpiryAt = this.nextExpiryAt;
    this.#fetchGeneration++;
    this.#deleteAllGeneration++;
    this.#pendingDeletionById.clear();
    this.loading = false;
    const mutationGeneration = this.#fetchGeneration;
    this.occurrences = [];
    this.notifications = [];
    this.unreadNotificationCount = 0;
    this.importantUnreadNotificationCount = 0;
    this.nextExpiryAt = null;

    try {
      await this.#api.deleteAllNotificationOccurrences();
    } catch (error) {
      if (this.#fetchGeneration === mutationGeneration) {
        this.occurrences = originalOccurrences;
        this.notifications = originalNotifications;
        this.unreadNotificationCount = originalCount;
        this.importantUnreadNotificationCount = originalImportantCount;
        this.nextExpiryAt = originalNextExpiryAt;
      }
      throw error;
    }
  }

  getPolicy(roomId?: string): Promise<NotificationPolicyItem[]> {
    return this.#api.getNotificationPolicy(roomId);
  }

  setPolicyPreference(
    reason: NotificationReason,
    intensity: NotificationDeliveryIntensity,
    roomId?: string
  ): Promise<NotificationPolicyItem[]> {
    return this.#api.setNotificationPolicyPreference(reason, intensity, roomId);
  }

  /**
   * Fetch the newest unread notification occurrence for a single room.
   *
   * Room sidebar badge clicks need the same scoped source as room notification
   * counts when the global cached page is empty, stale, or does not include
   * this room's notification.
   */
  async fetchRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): Promise<RoomNotificationLookup> {
    try {
      let offset = 0;
      let totalCount = 0;
      let notification: NotificationItem | null = null;
      let hasMore = false;
      do {
        const page = await this.#api.listNotificationOccurrences(50, offset);
        const matches = page.occurrences
          .filter((occurrence) => occurrence.unread && occurrence.room?.id === roomId)
          .map(occurrenceAsNotificationItem)
          .filter((item) => (options.isDM ? isDMNotification(item) : !isDMNotification(item)));
        totalCount += matches.length;
        if (!notification && matches.length > 0) notification = matches[0]!;
        hasMore = page.hasMore;
        if (!hasMore || page.occurrences.length === 0) break;
        offset += page.occurrences.length;
      } while (hasMore);
      if (notification) {
        this.#upsertNotification(notification);
      }

      return {
        ok: true,
        totalCount,
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
    return this.fetchRoomNotification(roomId, options);
  }

  /** Mark one occurrence read optimistically, then reconcile authoritative state. */
  async markRead(notificationId: string): Promise<boolean> {
    const removed = this.notifications.find((n) => n.id === notificationId);
    if (!removed) return false;

    // Supersede any in-flight list read without scheduling its generic retry;
    // this mutation performs one authoritative refresh after the write.
    this.#fetchGeneration++;
    this.loading = false;
    const originalOccurrences = this.occurrences;
    const originalCount = this.unreadNotificationCount;
    const originalImportantCount = this.importantUnreadNotificationCount;
    const occurrence = this.occurrences.find((candidate) => candidate.id === notificationId);
    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
    this.occurrences = this.occurrences.map((occurrence) =>
      occurrence.id === notificationId ? { ...occurrence, unread: false } : occurrence
    );
    this.unreadNotificationCount = Math.max(0, this.unreadNotificationCount - 1);
    if (occurrence?.attentionLevel === NotificationAttentionLevel.IMPORTANT) {
      this.importantUnreadNotificationCount = Math.max(
        0,
        this.importantUnreadNotificationCount - 1
      );
    }

    try {
      await this.#api.markNotificationRead(notificationId);
      return true;
    } catch (e) {
      console.error('Failed to mark notification read:', e);
      this.occurrences = originalOccurrences;
      this.#restoreNotification(removed);
      this.unreadNotificationCount = originalCount;
      this.importantUnreadNotificationCount = originalImportantCount;
      return false;
    }
  }

  /**
   * Re-insert a previously-removed notification, sorted most-recent-first by
   * createdAt to preserve the canonical ordering after a rollback.
   */
  #restoreNotification(notification: NotificationItem): void {
    this.#upsertNotification(notification);
  }

  #upsertNotification(notification: NotificationItem): boolean {
    const existed = this.notifications.some((candidate) => candidate.id === notification.id);
    this.#invalidateFetch();
    this.notifications = [
      ...this.notifications.filter((n) => n.id !== notification.id),
      notification
    ]
      .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
      .slice(0, 50);
    return !existed;
  }

  #invalidateFetch(): void {
    const shouldRestart = this.loading;
    this.#fetchGeneration++;
    this.loading = false;
    if (shouldRestart) {
      const invalidatedGeneration = this.#fetchGeneration;
      queueMicrotask(() => {
        if (this.#fetchGeneration === invalidatedGeneration && !this.loading) {
          void this.fetch();
        }
      });
    }
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
}

function redactedNotificationSummary(kind: NotificationItemKind): string {
  switch (kind) {
    case NotificationItemKind.DirectMessage:
      return 'New message';
    case NotificationItemKind.Mention:
      return 'You were mentioned';
    case NotificationItemKind.Reply:
      return 'New reply to your message';
    case NotificationItemKind.RoomMessage:
      return 'New message';
  }
}

function earliestNotificationOccurrenceExpiry(
  occurrences: NotificationOccurrenceItem[]
): string | null {
  return (
    occurrences
      .map((occurrence) => occurrence.expiresAt)
      .filter((expiry): expiry is string => Boolean(expiry))
      .sort()[0] ?? null
  );
}

function mergeNotificationOccurrences(
  current: NotificationOccurrenceItem[],
  restored: NotificationOccurrenceItem[]
): NotificationOccurrenceItem[] {
  const byId = new SvelteMap(current.map((occurrence) => [occurrence.id, occurrence]));
  for (const occurrence of restored) byId.set(occurrence.id, occurrence);
  return [...byId.values()].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

function mergeNotificationItems(
  current: NotificationItem[],
  restored: NotificationItem[]
): NotificationItem[] {
  const byId = new SvelteMap(current.map((notification) => [notification.id, notification]));
  for (const notification of restored) byId.set(notification.id, notification);
  return [...byId.values()].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 50);
}
