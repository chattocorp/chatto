import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import {
  NotificationAttentionLevel,
  type NotificationAPI,
  type NotificationOccurrenceItem,
  type NotificationOccurrencePage,
  type NotificationPolicy,
  type NotificationPolicyPatch,
  NotificationSignalKind
} from '$lib/api-client/notifications';
import { NotificationPolicyMatrixState } from './notificationPolicies.svelte';

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
  notification: NotificationOccurrenceItem | null;
};

export type RoomNotificationResolveOptions = {
  isDM?: boolean;
};

function isDMNotification(notification: NotificationOccurrenceItem): boolean {
  return notification.signalKind === NotificationSignalKind.DIRECT_MESSAGE;
}

/**
 * Extract the target a notification points to. Adding a new notification type
 * means updating this single function instead of every read site.
 */
export function notificationTarget(n: NotificationOccurrenceItem): NotificationTarget {
  return {
    isDM: isDMNotification(n),
    roomId: n.room?.id ?? null,
    roomName: n.room?.name ?? null,
    eventId: n.eventId || null,
    threadRootId: n.threadRootId
  };
}

/** Return the strongest unread notification attention for one exact thread. */
export function notificationAttentionForThread(
  notifications: readonly NotificationOccurrenceItem[],
  roomId: string,
  threadRootId: string
): NotificationAttentionLevel {
  let strongest = NotificationAttentionLevel.UNSPECIFIED;
  for (const notification of notifications) {
    if (!notification.unread) continue;
    const target = notificationTarget(notification);
    if (target.roomId !== roomId || target.threadRootId !== threadRootId) continue;
    if (notification.attentionLevel === NotificationAttentionLevel.IMPORTANT) {
      return NotificationAttentionLevel.IMPORTANT;
    }
    if (notification.attentionLevel === NotificationAttentionLevel.AMBIENT) {
      strongest = NotificationAttentionLevel.AMBIENT;
    }
  }
  return strongest;
}

/**
 * Notification state store.
 * Manages notifications for the current user with real-time sync.
 */
export class NotificationStore {
  #api: NotificationAPI;
  #fetchGeneration = 0;
  // Authoritative replacements suppress stale reads and optimistic state.
  #authoritativeGeneration = 0;
  // Exact deletions are independent and can overlap.
  #deletionSequence = 0;
  #pendingDeletionById = new SvelteMap<string, number>();
  #readSequence = 0;
  #pendingReadById = new SvelteMap<string, number>();
  #pendingReadRequestById = new SvelteMap<string, Promise<NotificationOccurrenceItem>>();
  #pendingMutationCount = 0;
  #mutationIdleWaiters = new SvelteSet<() => void>();
  #failedMutationReconciliation: Promise<void> | undefined;
  #firstPageRequest: Promise<NotificationOccurrencePage> | undefined;
  occurrences = $state.raw<NotificationOccurrenceItem[]>([]);
  /** Raw server rows consumed by the retained occurrence page. */
  consumedCount = $state(0);
  /** Exact visible occurrence total reported with the retained page. */
  totalCount = $state(0);
  /** Whether the retained page has older server rows available. */
  hasMore = $state(false);
  unreadNotificationCount = $state(0);
  importantUnreadNotificationCount = $state(0);
  roomUnreadCounts = $state.raw<Record<string, number>>({});
  roomImportantUnreadCounts = $state.raw<Record<string, number>>({});
  nextExpiryAt = $state<string | null>(null);
  readonly revokedRoomIds = new SvelteSet<string>();
  /** Users whose copied profile data must not be rendered from stale notification pages. */
  readonly scrubbedUserIds = new SvelteSet<string>();
  loading = $state(false);
  hasLoaded = $state(false);
  error = $state<string | null>(null);
  readonly notificationPolicies: NotificationPolicyMatrixState;

  constructor(api: NotificationAPI) {
    this.#api = api;
    this.notificationPolicies = new NotificationPolicyMatrixState(api);
  }

  get count() {
    return this.unreadOccurrences.length;
  }

  /** Loaded unread occurrences, newest first, for navigation indicators. */
  get unreadOccurrences(): NotificationOccurrenceItem[] {
    return this.occurrences
      .filter((occurrence) => occurrence.unread)
      .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
      .slice(0, 50);
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
    this.consumedCount = page.consumedCount ?? page.occurrences.length;
    this.totalCount = page.totalCount;
    this.hasMore = page.hasMore && this.consumedCount > 0;
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

  /** Invalidate projection-owned state while a compacted reset hydrates. */
  resetProjectionState(): void {
    this.notificationPolicies.reset();
    this.#fetchGeneration++;
    this.#authoritativeGeneration++;
    this.#pendingDeletionById.clear();
    this.#pendingReadById.clear();
    this.#pendingReadRequestById.clear();
    this.occurrences = [];
    this.consumedCount = 0;
    this.totalCount = 0;
    this.hasMore = false;
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
  }

  /** Remove copied profile data for an account deleted from the projection. */
  scrubUser(userId: string): void {
    this.scrubbedUserIds.add(userId);
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
    const roomOccurrenceIds = new SvelteSet(
      this.occurrences
        .filter((occurrence) => occurrence.room?.id === roomId)
        .map((occurrence) => occurrence.id)
    );
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
    this.consumedCount = this.occurrences.length;
    this.totalCount = Math.max(this.occurrences.length, this.totalCount - roomOccurrenceIds.size);

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
  }

  /** Re-open a room only after an explicit positive membership projection. */
  restoreRoom(roomId: string): void {
    if (!this.revokedRoomIds.delete(roomId)) return;
    this.#authoritativeGeneration++;
  }

  /**
   * Get the set of thread root IDs that have pending reply notifications.
   * Used to show notification indicators on thread buttons.
   */
  get threadsWithNotifications(): SvelteSet<string> {
    const threadIds = new SvelteSet<string>();
    for (const n of this.unreadOccurrences) {
      const threadRootId = notificationTarget(n).threadRootId;
      if (threadRootId) threadIds.add(threadRootId);
    }
    return threadIds;
  }

  /**
   * Check if a specific thread has unread notification occurrences.
   */
  hasThreadNotification(threadRootId: string): boolean {
    return this.unreadOccurrences.some((n) => notificationTarget(n).threadRootId === threadRootId);
  }

  /**
   * Check if a specific room has pending non-DM notifications.
   */
  hasRoomNotification(roomId: string): boolean {
    return this.unreadOccurrences.some((n) => {
      const t = notificationTarget(n);
      return !t.isDM && t.roomId === roomId;
    });
  }

  /** Check if the server has any pending non-DM notifications. */
  hasNonDMNotifications(): boolean {
    return this.unreadOccurrences.some((n) => !notificationTarget(n).isDM);
  }

  /**
   * Get the most recent non-DM notification.
   * Notifications are sorted most-recent-first, so .find returns the freshest.
   */
  getNonDMNotification(): NotificationOccurrenceItem | undefined {
    return this.unreadOccurrences.find((n) => n.targetSupported && !notificationTarget(n).isDM);
  }

  /**
   * Get the most recent non-DM notification for a room.
   */
  getRoomNotification(roomId: string): NotificationOccurrenceItem | undefined {
    return this.unreadOccurrences.find((n) => {
      const t = notificationTarget(n);
      return !t.isDM && t.roomId === roomId;
    });
  }

  /**
   * Check if there are any pending DM notifications.
   */
  hasDMNotifications(): boolean {
    return this.unreadOccurrences.some((n) => isDMNotification(n));
  }

  /**
   * Get the most recent DM notification.
   * Returns undefined if no DM notifications exist.
   */
  getDMNotification(): NotificationOccurrenceItem | undefined {
    return this.unreadOccurrences.find((n) => isDMNotification(n));
  }

  /**
   * Check if a specific DM conversation has unread notification occurrences.
   * Counterpart to {@link hasRoomNotification}, which excludes DMs.
   */
  hasDMRoomNotification(roomId: string): boolean {
    return this.unreadOccurrences.some((n) => isDMNotification(n) && n.room?.id === roomId);
  }

  /**
   * Get the most recent notification for a DM conversation.
   */
  getDMRoomNotification(roomId: string): NotificationOccurrenceItem | undefined {
    return this.unreadOccurrences.find((n) => isDMNotification(n) && n.room?.id === roomId);
  }

  getCachedRoomNotification(
    roomId: string,
    options: RoomNotificationResolveOptions = {}
  ): NotificationOccurrenceItem | undefined {
    return options.isDM ? this.getDMRoomNotification(roomId) : this.getRoomNotification(roomId);
  }

  /**
   * Fetch all notifications from the server.
   *
   * Resilience contract: a server-side error (e.g. a schema mismatch on a
   * remote instance running an older backend, network failure, transient
   * 500) records the error message and logs it, but leaves
   * `this.occurrences` at its previous value. This matters in
   * multi-instance setups — the bell, DM dot, etc. aggregate across
   * NotificationStore instances, and one bad response on one instance
   * must not erase already-loaded notifications on others.
   */
  async fetch() {
    await this.#refresh(true);
  }

  /** Quietly reconcile after a potentially missed best-effort live hint. */
  async reconcile() {
    await this.#refresh(false);
  }

  async #refresh(showLoading: boolean) {
    if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
    const generation = ++this.#fetchGeneration;
    if (showLoading) this.loading = true;
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
      if (showLoading && generation === this.#fetchGeneration) {
        this.loading = false;
      }
    }
  }

  async fetchPage(offset = 0): Promise<NotificationOccurrencePage> {
    if (offset !== 0) return await this.#fetchPage(offset);
    if (this.#firstPageRequest) return await this.#firstPageRequest;
    const request = this.#fetchPage(0);
    this.#firstPageRequest = request;
    try {
      return await request;
    } finally {
      if (this.#firstPageRequest === request) this.#firstPageRequest = undefined;
    }
  }

  async #fetchPage(offset: number): Promise<NotificationOccurrencePage> {
    if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
    const authoritativeGeneration = this.#authoritativeGeneration;
    const page = await this.#api.listNotificationOccurrences(50, offset);
    if (
      authoritativeGeneration !== this.#authoritativeGeneration ||
      this.#pendingMutationCount > 0
    ) {
      if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
      return await this.#fetchPage(offset);
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
      const consumedCount = safePage.consumedCount ?? safePage.occurrences.length;
      this.consumedCount = Math.max(this.consumedCount, offset + consumedCount);
      this.totalCount = safePage.totalCount;
      this.hasMore = safePage.hasMore && consumedCount > 0;
    }
    return safePage;
  }

  async markOccurrenceRead(notificationId: string): Promise<void> {
    if (!(await this.markRead(notificationId))) {
      throw new Error('Failed to mark notification read');
    }
  }

  /** Delete exact occurrences optimistically, reconciling ambiguous failures. */
  async deleteOccurrences(
    notificationIds: string[],
    knownCounts?: { unread: number; importantUnread: number; roomId?: string | null }
  ): Promise<void> {
    const uniqueIds = [...new SvelteSet(notificationIds)];
    const removedIds = new SvelteSet(uniqueIds);
    const removedOccurrences = this.occurrences.filter((occurrence) =>
      removedIds.has(occurrence.id)
    );
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
    this.consumedCount = Math.max(0, this.consumedCount - removedOccurrences.length);
    this.totalCount = Math.max(0, this.totalCount - removedOccurrences.length);
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

    let mutationFailed = false;
    let mutationError: unknown;
    try {
      await this.#api.batchDeleteNotificationOccurrences(uniqueIds);
      // Keep the deletion marker in place until older reads settle. A failed
      // read must never restore an occurrence after its delete committed.
      if (overlappingReadRequests.length > 0) {
        await Promise.allSettled(overlappingReadRequests);
      }
    } catch (error) {
      mutationFailed = true;
      mutationError = error;
    } finally {
      for (const id of uniqueIds) {
        if (this.#pendingDeletionById.get(id) === mutation) {
          this.#pendingDeletionById.delete(id);
        }
      }
      this.#endMutation();
    }
    if (mutationFailed) {
      await this.#reconcileAfterFailedMutation(mutationError);
      throw mutationError;
    }
  }

  /** Delete every current occurrence optimistically for this server. */
  async deleteAllOccurrences(): Promise<void> {
    this.#fetchGeneration++;
    this.#beginMutation();
    this.#pendingDeletionById.clear();
    this.loading = false;
    this.occurrences = [];
    this.consumedCount = 0;
    this.totalCount = 0;
    this.hasMore = false;
    this.unreadNotificationCount = 0;
    this.importantUnreadNotificationCount = 0;
    this.roomUnreadCounts = {};
    this.roomImportantUnreadCounts = {};
    this.nextExpiryAt = null;

    let mutationFailed = false;
    let mutationError: unknown;
    try {
      await this.#api.deleteAllNotificationOccurrences();
    } catch (error) {
      mutationFailed = true;
      mutationError = error;
    } finally {
      this.#endMutation();
    }
    if (mutationFailed) {
      await this.#reconcileAfterFailedMutation(mutationError);
      throw mutationError;
    }
  }

  getPolicy(roomId?: string): Promise<NotificationPolicy> {
    return this.#api.getNotificationPolicy(roomId);
  }

  updatePolicy(patch: NotificationPolicyPatch, roomId?: string): Promise<NotificationPolicy> {
    return this.#api.updateNotificationPolicy(patch, roomId);
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
          if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
          return await this.fetchRoomNotification(roomId, options);
        }
        if (this.revokedRoomIds.has(roomId)) {
          return { ok: true, totalCount: 0, notification: null };
        }
        const matches = page.occurrences
          .filter((occurrence) => occurrence.unread && occurrence.room?.id === roomId)
          .filter((occurrence) =>
            options.isDM ? isDMNotification(occurrence) : !isDMNotification(occurrence)
          );
        totalCount += matches.length;
        if (!matchedOccurrence && matches.length > 0) matchedOccurrence = matches[0]!;
        hasMore = page.hasMore;
        const consumedCount = page.consumedCount ?? page.occurrences.length;
        if (!hasMore || consumedCount === 0) break;
        offset += consumedCount;
      } while (hasMore);
      if (this.revokedRoomIds.has(roomId)) {
        return { ok: true, totalCount: 0, notification: null };
      }
      const notification = matchedOccurrence
        ? this.#privacySafeOccurrence(matchedOccurrence)
        : null;
      if (matchedOccurrence && notification) {
        this.occurrences = mergeNotificationOccurrences(this.occurrences, [
          this.#privacySafeOccurrence(matchedOccurrence)
        ]);
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
    const occurrence = this.occurrences.find((candidate) => candidate.id === notificationId);
    if (!occurrence) {
      this.#fetchGeneration++;
      this.#beginMutation();
      let mutationFailed = false;
      let mutationError: unknown;
      try {
        await this.#api.markNotificationRead(notificationId);
      } catch (error) {
        mutationFailed = true;
        mutationError = error;
      } finally {
        this.#endMutation();
      }
      if (mutationFailed) {
        await this.#reconcileAfterFailedMutation(mutationError);
        throw mutationError;
      }
      return true;
    }
    if (occurrence && !occurrence.unread) return true;

    // Supersede any in-flight list read without scheduling its generic retry;
    // this mutation performs one authoritative refresh after the write.
    this.#fetchGeneration++;
    this.#beginMutation();
    this.loading = false;
    const mutation = ++this.#readSequence;
    this.#pendingReadById.set(notificationId, mutation);
    const unreadDelta = occurrence.unread ? 1 : 0;
    const importantDelta =
      occurrence?.attentionLevel === NotificationAttentionLevel.IMPORTANT ? 1 : 0;
    const roomAdjustments = occurrence
      ? notificationRoomAdjustments([occurrence])
      : new SvelteMap<string, { unread: number; importantUnread: number }>();
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
    let mutationFailed = false;
    try {
      request = this.#api.markNotificationRead(notificationId);
      this.#pendingReadRequestById.set(notificationId, request);
      await request;
      return true;
    } catch (e) {
      console.error('Failed to mark notification read:', e);
      mutationFailed = true;
    } finally {
      if (this.#pendingReadById.get(notificationId) === mutation) {
        this.#pendingReadById.delete(notificationId);
      }
      if (request && this.#pendingReadRequestById.get(notificationId) === request) {
        this.#pendingReadRequestById.delete(notificationId);
      }
      this.#endMutation();
    }
    if (mutationFailed) {
      await this.#reconcileAfterFailedMutation();
      return false;
    }
    return true;
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

  async #reconcileAfterFailedMutation(originalError?: unknown): Promise<void> {
    if (!this.#failedMutationReconciliation) {
      const reconciliation = this.#runFailedMutationReconciliation();
      this.#failedMutationReconciliation = reconciliation;
      const clearReconciliation = () => {
        if (this.#failedMutationReconciliation === reconciliation) {
          this.#failedMutationReconciliation = undefined;
        }
      };
      void reconciliation.then(clearReconciliation, clearReconciliation);
    }
    try {
      await this.#failedMutationReconciliation;
    } catch (error) {
      console.error('Failed to reconcile notifications after an ambiguous mutation:', error, {
        cause: originalError
      });
    }
  }

  #privacySafeOccurrence(occurrence: NotificationOccurrenceItem): NotificationOccurrenceItem {
    return occurrence.actor && this.scrubbedUserIds.has(occurrence.actor.id)
      ? { ...occurrence, actor: null }
      : occurrence;
  }

  async #runFailedMutationReconciliation(): Promise<void> {
    while (true) {
      if (this.#pendingMutationCount > 0) await this.#waitForPendingMutations();
      const authoritativeGeneration = this.#authoritativeGeneration;
      const page = await this.#api.listNotificationOccurrences(50, 0);
      if (
        this.#pendingMutationCount > 0 ||
        authoritativeGeneration !== this.#authoritativeGeneration
      ) {
        continue;
      }
      this.replaceOccurrenceProjection(page);
      return;
    }
  }

  /**
   * Get location string for a notification (e.g., "#general in My Server").
   * Returns null for DM notifications and any notification missing names.
   * The "in <name>" suffix uses the connected instance display name supplied
   * by the caller.
   */
  getLocationString(
    notification: NotificationOccurrenceItem,
    serverName?: string | null
  ): string | null {
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
  getCleanPath(serverId: string, notification: NotificationOccurrenceItem): string {
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
