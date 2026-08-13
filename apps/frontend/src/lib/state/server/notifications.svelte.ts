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
  #readSequence = 0;
  #pendingReadById = new SvelteMap<string, number>();
  #pendingReadRequestById = new SvelteMap<string, Promise<NotificationOccurrenceItem>>();
  #pendingMutationCount = 0;
  #mutationIdleWaiters = new Set<() => void>();
  notifications = $state<NotificationItem[]>([]);
  occurrences = $state.raw<NotificationOccurrenceItem[]>([]);
  unreadNotificationCount = $state(0);
  importantUnreadNotificationCount = $state(0);
  roomUnreadCounts = $state.raw<Record<string, number>>({});
  roomImportantUnreadCounts = $state.raw<Record<string, number>>({});
  nextExpiryAt = $state<string | null>(null);
  /** Advances only for realtime invalidations, including changes made in another session. */
  viewInvalidationVersion = $state(0);
  resetVersion = $state(0);
  readonly revokedRoomIds = new SvelteSet<string>();
  /** Users whose copied profile data must not be rendered from stale notification pages. */
  readonly scrubbedUserIds = new SvelteSet<string>();
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
    this.#pendingReadById.clear();
    this.#pendingReadRequestById.clear();
    const occurrences = page.occurrences
      .filter((occurrence) => !occurrence.room || !this.revokedRoomIds.has(occurrence.room.id))
      .map((occurrence) => this.#privacySafeOccurrence(occurrence));
    const revokedUnreadCount = [...this.revokedRoomIds].reduce(
      (total, roomId) => total + (page.roomUnreadCounts[roomId] ?? 0),
      0
    );
    const revokedImportantUnreadCount = [...this.revokedRoomIds].reduce(
      (total, roomId) => total + (page.roomImportantUnreadCounts[roomId] ?? 0),
      0
    );
    this.occurrences = occurrences;
    this.notifications = occurrences
      .filter((occurrence) => occurrence.unread)
      .map(occurrenceAsNotificationItem)
      .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
      .slice(0, 50);
    this.unreadNotificationCount = Math.max(0, page.unreadCount - revokedUnreadCount);
    this.importantUnreadNotificationCount = Math.max(
      0,
      Math.min(
        page.importantUnreadCount - revokedImportantUnreadCount,
        this.unreadNotificationCount
      )
    );
    this.roomUnreadCounts = omitRecordKeys(page.roomUnreadCounts, this.revokedRoomIds);
    this.roomImportantUnreadCounts = omitRecordKeys(
      page.roomImportantUnreadCounts,
      this.revokedRoomIds
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
    this.#pendingReadById.clear();
    this.#pendingReadRequestById.clear();
    this.notifications = [];
    this.occurrences = [];
    this.unreadNotificationCount = 0;
    this.importantUnreadNotificationCount = 0;
    this.roomUnreadCounts = {};
    this.roomImportantUnreadCounts = {};
    this.nextExpiryAt = null;
    this.loading = true;
    // The empty reset boundary is already authoritative. Keep this true so
    // badge synchronisation clears stale native notification counts even when
    // a later snapshot frame never arrives.
    this.hasLoaded = true;
    this.error = null;
    this.resetVersion++;
    this.invalidateViews();
  }

  /** Remove copied profile data for an account deleted from the projection. */
  scrubUser(userId: string): void {
    this.scrubbedUserIds.add(userId);
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
    this.invalidateViews();
  }

  /** Drop notification payloads for a room at an authorization boundary. */
  clearRoom(roomId: string): void {
    this.#authoritativeGeneration++;
    const roomOccurrenceIds = new SvelteSet(
      this.occurrences
        .filter((occurrence) => occurrence.room?.id === roomId)
        .map((occurrence) => occurrence.id)
    );
    for (const notification of this.notifications) {
      if (notificationTarget(notification).roomId === roomId)
        roomOccurrenceIds.add(notification.id);
    }
    for (const id of roomOccurrenceIds) {
      this.#pendingDeletionById.delete(id);
      this.#pendingReadById.delete(id);
      this.#pendingReadRequestById.delete(id);
    }
    this.revokedRoomIds.add(roomId);
    const removedUnreadOccurrences =
      this.roomUnreadCounts[roomId] ??
      this.occurrences.filter((occurrence) => occurrence.unread && occurrence.room?.id === roomId)
        .length;
    const removedImportantUnreadOccurrences =
      this.roomImportantUnreadCounts[roomId] ??
      this.occurrences.filter(
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
    this.roomUnreadCounts = withoutRecordKey(this.roomUnreadCounts, roomId);
    this.roomImportantUnreadCounts = withoutRecordKey(this.roomImportantUnreadCounts, roomId);
    this.nextExpiryAt = earliestNotificationOccurrenceExpiry(this.occurrences);
    this.invalidateViews();
  }

  /** Re-open a room only after an explicit positive membership projection. */
  restoreRoom(roomId: string): void {
    if (!this.revokedRoomIds.delete(roomId)) return;
    this.invalidateViews();
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
    if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
    const generation = ++this.#fetchGeneration;
    this.loading = true;
    this.error = null;

    try {
      const page = await this.#api.listNotificationOccurrences(50);
      if (generation !== this.#fetchGeneration || this.#pendingMutationCount > 0) return;

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
    if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
    const authoritativeGeneration = this.#authoritativeGeneration;
    const page = await this.#api.listNotificationOccurrences(50, offset);
    if (
      authoritativeGeneration !== this.#authoritativeGeneration ||
      this.#pendingMutationCount > 0
    ) {
      throw new Error('Notification projection changed while loading');
    }
    const revokedUnreadCount = [...this.revokedRoomIds].reduce(
      (total, roomId) => total + (page.roomUnreadCounts[roomId] ?? 0),
      0
    );
    const revokedImportantUnreadCount = [...this.revokedRoomIds].reduce(
      (total, roomId) => total + (page.roomImportantUnreadCounts[roomId] ?? 0),
      0
    );
    const safePage = {
      ...page,
      consumedCount: page.consumedCount ?? page.occurrences.length,
      occurrences: page.occurrences
        .filter((occurrence) => !occurrence.room || !this.revokedRoomIds.has(occurrence.room.id))
        .map((occurrence) => this.#privacySafeOccurrence(occurrence)),
      unreadCount: Math.max(0, page.unreadCount - revokedUnreadCount),
      importantUnreadCount: Math.max(
        0,
        Math.min(
          page.importantUnreadCount - revokedImportantUnreadCount,
          page.unreadCount - revokedUnreadCount
        )
      ),
      roomUnreadCounts: omitRecordKeys(page.roomUnreadCounts, this.revokedRoomIds),
      roomImportantUnreadCounts: omitRecordKeys(page.roomImportantUnreadCounts, this.revokedRoomIds)
    };
    if (offset === 0) this.replaceOccurrenceProjection(safePage);
    else {
      this.occurrences = mergeNotificationOccurrences(this.occurrences, safePage.occurrences);
    }
    return safePage;
  }

  async markOccurrenceRead(notificationId: string): Promise<void> {
    if (!(await this.markRead(notificationId))) {
      throw new Error('Failed to mark notification read');
    }
  }

  /** Delete exact occurrences optimistically, restoring them on failure. */
  async deleteOccurrences(
    notificationIds: string[],
    knownCounts?: { unread: number; importantUnread: number; roomId?: string | null }
  ): Promise<void> {
    const uniqueIds = [...new SvelteSet(notificationIds)];
    const removedIds = new SvelteSet(uniqueIds);
    const removedOccurrences = this.occurrences.filter((occurrence) =>
      removedIds.has(occurrence.id)
    );
    const removedNotifications = this.notifications.filter((notification) =>
      removedIds.has(notification.id)
    );
    const deleteAllGeneration = this.#deleteAllGeneration;
    const mutation = ++this.#deletionSequence;
    const overlappingReadRequests = uniqueIds.flatMap((id) => {
      const request = this.#pendingReadRequestById.get(id);
      return request ? [request] : [];
    });
    this.#beginMutation();
    for (const id of uniqueIds) {
      this.#pendingDeletionById.set(id, mutation);
    }
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
          occurrence.unread && occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT
      ).length;
    this.unreadNotificationCount = Math.max(0, this.unreadNotificationCount - removedUnreadCount);
    this.importantUnreadNotificationCount = Math.max(
      0,
      this.importantUnreadNotificationCount - removedImportantUnreadCount
    );
    const roomAdjustments = notificationRoomAdjustments(removedOccurrences, knownCounts);
    this.#adjustRoomCounts(roomAdjustments, -1);
    this.nextExpiryAt = earliestNotificationOccurrenceExpiry(this.occurrences);

    try {
      await this.#api.batchDeleteNotificationOccurrences(uniqueIds);
      // Keep the deletion marker in place until older reads settle. A failed
      // read must never restore an occurrence after its delete committed.
      if (overlappingReadRequests.length > 0) {
        await Promise.allSettled(overlappingReadRequests);
      }
    } catch (error) {
      const rollbackIds = new SvelteSet(
        uniqueIds.filter((id) => this.#pendingDeletionById.get(id) === mutation)
      );
      if (this.#deleteAllGeneration === deleteAllGeneration && rollbackIds.size > 0) {
        if (overlappingReadRequests.length > 0) {
          await Promise.allSettled(overlappingReadRequests);
          await this.#reconcileAfterOverlappingMutation();
          throw error;
        }
        const visibleRollbackIds = new SvelteSet(
          [...rollbackIds].filter((id) => {
            const occurrence = removedOccurrences.find((candidate) => candidate.id === id);
            const roomId = occurrence?.room?.id;
            return !roomId || !this.revokedRoomIds.has(roomId);
          })
        );
        this.occurrences = mergeNotificationOccurrences(
          this.occurrences,
          removedOccurrences
            .filter((occurrence) => visibleRollbackIds.has(occurrence.id))
            .map((occurrence) => this.#privacySafeOccurrence(occurrence))
        );
        this.notifications = mergeNotificationItems(
          this.notifications,
          removedNotifications
            .filter((notification) => visibleRollbackIds.has(notification.id))
            .map((notification) => this.#privacySafeNotification(notification))
        );
        const rollbackUnreadCount =
          knownCounts !== undefined && visibleRollbackIds.size === removedIds.size
            ? knownCounts.unread
            : removedOccurrences.filter(
                (occurrence) => visibleRollbackIds.has(occurrence.id) && occurrence.unread
              ).length;
        const rollbackImportantUnreadCount =
          knownCounts !== undefined && visibleRollbackIds.size === removedIds.size
            ? knownCounts.importantUnread
            : removedOccurrences.filter(
                (occurrence) =>
                  visibleRollbackIds.has(occurrence.id) &&
                  occurrence.unread &&
                  occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT
              ).length;
        this.unreadNotificationCount += rollbackUnreadCount;
        this.importantUnreadNotificationCount += rollbackImportantUnreadCount;
        const rollbackRoomAdjustments =
          knownCounts !== undefined && visibleRollbackIds.size === removedIds.size
            ? notificationRoomAdjustments([], knownCounts)
            : notificationRoomAdjustments(
                removedOccurrences.filter((occurrence) => visibleRollbackIds.has(occurrence.id))
              );
        this.#adjustRoomCounts(rollbackRoomAdjustments, 1);
        this.nextExpiryAt = earliestNotificationOccurrenceExpiry(this.occurrences);
      }
      throw error;
    } finally {
      for (const id of uniqueIds) {
        if (this.#pendingDeletionById.get(id) === mutation) {
          this.#pendingDeletionById.delete(id);
        }
      }
      this.#endMutation();
    }
  }

  /** Delete every current occurrence optimistically for this server. */
  async deleteAllOccurrences(): Promise<void> {
    const originalOccurrences = this.occurrences;
    const originalNotifications = this.notifications;
    const originalCount = this.unreadNotificationCount;
    const originalImportantCount = this.importantUnreadNotificationCount;
    const originalRoomUnreadCounts = this.roomUnreadCounts;
    const originalRoomImportantUnreadCounts = this.roomImportantUnreadCounts;
    const originalNextExpiryAt = this.nextExpiryAt;
    const overlappingReadRequests = [...this.#pendingReadRequestById.values()];
    this.#fetchGeneration++;
    this.#beginMutation();
    this.#deleteAllGeneration++;
    this.#pendingDeletionById.clear();
    this.loading = false;
    const mutationGeneration = this.#fetchGeneration;
    this.occurrences = [];
    this.notifications = [];
    this.unreadNotificationCount = 0;
    this.importantUnreadNotificationCount = 0;
    this.roomUnreadCounts = {};
    this.roomImportantUnreadCounts = {};
    this.nextExpiryAt = null;

    try {
      await this.#api.deleteAllNotificationOccurrences();
    } catch (error) {
      if (this.#fetchGeneration === mutationGeneration) {
        if (overlappingReadRequests.length > 0) {
          await Promise.allSettled(overlappingReadRequests);
          await this.#reconcileAfterOverlappingMutation();
          throw error;
        }
        const revokedUnreadCount = [...this.revokedRoomIds].reduce(
          (total, roomId) => total + (originalRoomUnreadCounts[roomId] ?? 0),
          0
        );
        const revokedImportantUnreadCount = [...this.revokedRoomIds].reduce(
          (total, roomId) => total + (originalRoomImportantUnreadCounts[roomId] ?? 0),
          0
        );
        this.occurrences = originalOccurrences
          .filter((occurrence) => !occurrence.room || !this.revokedRoomIds.has(occurrence.room.id))
          .map((occurrence) => this.#privacySafeOccurrence(occurrence));
        this.notifications = originalNotifications
          .filter((notification) => {
            const roomId = notificationTarget(notification).roomId;
            return !roomId || !this.revokedRoomIds.has(roomId);
          })
          .map((notification) => this.#privacySafeNotification(notification));
        this.unreadNotificationCount = Math.max(0, originalCount - revokedUnreadCount);
        this.importantUnreadNotificationCount = Math.max(
          0,
          originalImportantCount - revokedImportantUnreadCount
        );
        this.roomUnreadCounts = omitRecordKeys(originalRoomUnreadCounts, this.revokedRoomIds);
        this.roomImportantUnreadCounts = omitRecordKeys(
          originalRoomImportantUnreadCounts,
          this.revokedRoomIds
        );
        this.nextExpiryAt = this.revokedRoomIds.size
          ? earliestNotificationOccurrenceExpiry(this.occurrences)
          : originalNextExpiryAt;
      }
      throw error;
    } finally {
      this.#endMutation();
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
      if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
      const authoritativeGeneration = this.#authoritativeGeneration;
      if (this.revokedRoomIds.has(roomId)) {
        return { ok: true, totalCount: 0, notification: null };
      }
      let offset = 0;
      let totalCount = 0;
      let matchedOccurrence: NotificationOccurrenceItem | null = null;
      let hasMore = false;
      do {
        const page = await this.#api.listNotificationOccurrences(50, offset);
        if (
          authoritativeGeneration !== this.#authoritativeGeneration ||
          this.#pendingMutationCount > 0
        ) {
          return { ok: false, totalCount: null, notification: null };
        }
        if (this.revokedRoomIds.has(roomId)) {
          return { ok: true, totalCount: 0, notification: null };
        }
        const matches = page.occurrences
          .filter((occurrence) => occurrence.unread && occurrence.room?.id === roomId)
          .filter((occurrence) => {
            const item = occurrenceAsNotificationItem(occurrence);
            return options.isDM ? isDMNotification(item) : !isDMNotification(item);
          });
        totalCount += matches.length;
        if (!matchedOccurrence && matches.length > 0) matchedOccurrence = matches[0]!;
        hasMore = page.hasMore;
        if (!hasMore || page.occurrences.length === 0) break;
        offset += page.occurrences.length;
      } while (hasMore);
      if (this.revokedRoomIds.has(roomId)) {
        return { ok: true, totalCount: 0, notification: null };
      }
      const notification = matchedOccurrence
        ? this.#privacySafeNotification(
            occurrenceAsNotificationItem(this.#privacySafeOccurrence(matchedOccurrence))
          )
        : null;
      if (matchedOccurrence && notification) {
        this.occurrences = mergeNotificationOccurrences(this.occurrences, [
          this.#privacySafeOccurrence(matchedOccurrence)
        ]);
        this.#upsertNotification(this.#privacySafeNotification(notification));
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
    const occurrence = this.occurrences.find((candidate) => candidate.id === notificationId);
    if (!removed && !occurrence) {
      this.#fetchGeneration++;
      this.#beginMutation();
      try {
        await this.#api.markNotificationRead(notificationId);
        return true;
      } finally {
        this.#endMutation();
      }
    }
    if (occurrence && !occurrence.unread) return true;

    // Supersede any in-flight list read without scheduling its generic retry;
    // this mutation performs one authoritative refresh after the write.
    this.#fetchGeneration++;
    this.#beginMutation();
    this.loading = false;
    const deleteAllGeneration = this.#deleteAllGeneration;
    const mutation = ++this.#readSequence;
    this.#pendingReadById.set(notificationId, mutation);
    const unreadDelta = occurrence?.unread || removed ? 1 : 0;
    const importantDelta =
      occurrence?.attentionLevel === NotificationAttentionLevel.IMPORTANT ? 1 : 0;
    const roomAdjustments = occurrence
      ? notificationRoomAdjustments([occurrence])
      : new SvelteMap<string, { unread: number; importantUnread: number }>();
    this.notifications = this.notifications.filter((n) => n.id !== notificationId);
    this.occurrences = this.occurrences.map((occurrence) =>
      occurrence.id === notificationId ? { ...occurrence, unread: false } : occurrence
    );
    this.unreadNotificationCount = Math.max(0, this.unreadNotificationCount - unreadDelta);
    this.importantUnreadNotificationCount = Math.max(
      0,
      this.importantUnreadNotificationCount - importantDelta
    );
    this.#adjustRoomCounts(roomAdjustments, -1);

    let request: Promise<NotificationOccurrenceItem> | undefined;
    try {
      request = this.#api.markNotificationRead(notificationId);
      this.#pendingReadRequestById.set(notificationId, request);
      await request;
      return true;
    } catch (e) {
      console.error('Failed to mark notification read:', e);
      if (
        this.#pendingReadById.get(notificationId) === mutation &&
        !this.#pendingDeletionById.has(notificationId) &&
        this.#deleteAllGeneration === deleteAllGeneration
      ) {
        if (occurrence && !this.revokedRoomIds.has(occurrence.room?.id ?? '')) {
          this.occurrences = mergeNotificationOccurrences(this.occurrences, [
            this.#privacySafeOccurrence(occurrence)
          ]);
          if (removed) this.#restoreNotification(this.#privacySafeNotification(removed));
          this.unreadNotificationCount += unreadDelta;
          this.importantUnreadNotificationCount += importantDelta;
          this.#adjustRoomCounts(roomAdjustments, 1);
        } else if (!occurrence && removed) {
          const roomId = notificationTarget(removed).roomId;
          if (!roomId || !this.revokedRoomIds.has(roomId)) {
            this.#restoreNotification(this.#privacySafeNotification(removed));
            this.unreadNotificationCount += unreadDelta;
          }
        }
      }
      return false;
    } finally {
      if (this.#pendingReadById.get(notificationId) === mutation) {
        this.#pendingReadById.delete(notificationId);
      }
      if (request && this.#pendingReadRequestById.get(notificationId) === request) {
        this.#pendingReadRequestById.delete(notificationId);
      }
      this.#endMutation();
    }
  }

  #beginMutation(): void {
    this.#pendingMutationCount++;
    this.#authoritativeGeneration++;
  }

  #endMutation(): void {
    this.#pendingMutationCount = Math.max(0, this.#pendingMutationCount - 1);
    if (this.#pendingMutationCount !== 0) return;
    for (const resolve of this.#mutationIdleWaiters) resolve();
    this.#mutationIdleWaiters.clear();
  }

  async #waitForPendingMutations(): Promise<void> {
    if (this.#pendingMutationCount === 0) return;
    await new Promise<void>((resolve) => this.#mutationIdleWaiters.add(resolve));
  }

  #adjustRoomCounts(
    adjustments: Map<string, { unread: number; importantUnread: number }>,
    direction: 1 | -1
  ): void {
    let unread = this.roomUnreadCounts;
    let important = this.roomImportantUnreadCounts;
    for (const [roomId, counts] of adjustments) {
      unread = withAdjustedRecordValue(unread, roomId, direction * counts.unread);
      important = withAdjustedRecordValue(important, roomId, direction * counts.importantUnread);
    }
    this.roomUnreadCounts = unread;
    this.roomImportantUnreadCounts = important;
  }

  async #reconcileAfterOverlappingMutation(): Promise<void> {
    const authoritativeGeneration = this.#authoritativeGeneration;
    const page = await this.#api.listNotificationOccurrences(50, 0);
    if (authoritativeGeneration !== this.#authoritativeGeneration) return;
    this.replaceOccurrenceProjection(page);
    this.invalidateViews();
  }

  #privacySafeOccurrence(occurrence: NotificationOccurrenceItem): NotificationOccurrenceItem {
    return occurrence.actor && this.scrubbedUserIds.has(occurrence.actor.id)
      ? { ...occurrence, actor: null }
      : occurrence;
  }

  #privacySafeNotification(notification: NotificationItem): NotificationItem {
    return notification.actor && this.scrubbedUserIds.has(notification.actor.id)
      ? {
          ...notification,
          actor: null,
          summary: redactedNotificationSummary(notification.kind)
        }
      : notification;
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

function withoutRecordKey(source: Record<string, number>, key: string): Record<string, number> {
  if (!(key in source)) return source;
  const next = { ...source };
  delete next[key];
  return next;
}

function omitRecordKeys(
  source: Record<string, number>,
  keys: ReadonlySet<string>
): Record<string, number> {
  let next = source;
  for (const key of keys) next = withoutRecordKey(next, key);
  return next === source ? { ...source } : next;
}

function withAdjustedRecordValue(
  source: Record<string, number>,
  key: string,
  delta: number
): Record<string, number> {
  const value = Math.max(0, (source[key] ?? 0) + delta);
  if (value === 0) return withoutRecordKey(source, key);
  return { ...source, [key]: value };
}

function notificationRoomAdjustments(
  occurrences: NotificationOccurrenceItem[],
  knownCounts?: { unread: number; importantUnread: number; roomId?: string | null }
): Map<string, { unread: number; importantUnread: number }> {
  if (knownCounts?.roomId) {
    return new SvelteMap([
      [
        knownCounts.roomId,
        { unread: knownCounts.unread, importantUnread: knownCounts.importantUnread }
      ]
    ]);
  }
  const result = new SvelteMap<string, { unread: number; importantUnread: number }>();
  for (const occurrence of occurrences) {
    const roomId = occurrence.room?.id;
    if (!roomId || !occurrence.unread) continue;
    const current = result.get(roomId) ?? { unread: 0, importantUnread: 0 };
    current.unread++;
    if (occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT) {
      current.importantUnread++;
    }
    result.set(roomId, current);
  }
  return result;
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
