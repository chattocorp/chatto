import { ReactiveMap, ReactiveSet, signal } from '../reactivity/index.js';
import {
  NotificationAttentionLevel,
  type NotificationAPI,
  type NotificationOccurrenceItem,
  type NotificationOccurrencePage,
  type NotificationPolicy,
  type NotificationPolicyPatch,
  NotificationSignalKind
} from '../api/notifications.js';

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
  /**
   * Restrict the lookup to one attention level. `IMPORTANT` matches only
   * Important occurrences; any other level matches every occurrence that is
   * not Important. This mirrors the room badges, which show the Important
   * count and the remaining count separately.
   */
  attentionLevel?: NotificationAttentionLevel;
};

/** Whether an occurrence belongs to a direct message. */
export function isDMNotification(notification: NotificationOccurrenceItem): boolean {
  return notification.signalKind === NotificationSignalKind.DIRECT_MESSAGE;
}

/** Whether an occurrence belongs to a room lookup with the given options. */
export function matchesRoomNotification(
  notification: NotificationOccurrenceItem,
  roomId: string,
  options: RoomNotificationResolveOptions
): boolean {
  if (notification.room?.id !== roomId) return false;
  if (isDMNotification(notification) !== Boolean(options.isDM)) return false;
  if (options.attentionLevel === undefined) return true;
  const important = notification.attentionLevel === NotificationAttentionLevel.IMPORTANT;
  return important === (options.attentionLevel === NotificationAttentionLevel.IMPORTANT);
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

/** Missing IDs are handled only when a complete page or zero unread count proves it. */
function handledIdsFromPage(
  ids: readonly string[],
  page: Pick<NotificationOccurrencePage, 'occurrences' | 'hasMore' | 'unreadCount'>,
  handled: Set<string>
): Set<string> {
  const rows = new Map(page.occurrences.map((row) => [row.id, row]));
  for (const id of ids) {
    const row = rows.get(id);
    if (row ? !row.unread : !page.hasMore || page.unreadCount === 0) handled.add(id);
  }
  return handled;
}

/**
 * The viewer's notification occurrences and counts on one server, kept
 * current by the realtime projection, with optimistic reads and deletions.
 *
 * The counts are the server's. A host that hides notifications for content
 * that the user looks at derives its own counts from these.
 */
export class NotificationStore {
  #api: NotificationAPI;
  #fetchGeneration = 0;
  // Authoritative replacements suppress stale reads and optimistic state.
  #authoritativeGeneration = 0;
  // Exact deletions are independent and can overlap.
  #deletionSequence = 0;
  #pendingDeletionById = new ReactiveMap<string, number>();
  #readSequence = 0;
  #pendingReadById = new ReactiveMap<string, number>();
  #pendingReadRequestById = new ReactiveMap<string, Promise<NotificationOccurrenceItem>>();
  readonly #pendingMutationCountSignal = signal(0);
  get #pendingMutationCount() {
    return this.#pendingMutationCountSignal.get();
  }
  set #pendingMutationCount(value) {
    this.#pendingMutationCountSignal.set(value);
  }
  #mutationIdleWaiters = new Set<() => void>();
  #failedMutationReconciliation: Promise<void> | undefined;
  #firstPageRequest: Promise<NotificationOccurrencePage> | undefined;
  #reconciliation: Promise<void> | undefined;
  #lastSuccessfulReadAt: number | null = null;
  #handledPushIds = new ReactiveSet<string>();
  /** Changes when native notification cleanup must recheck server state. */
  readonly #pushRevisionSignal = signal(0);
  get pushRevision() {
    return this.#pushRevisionSignal.get();
  }
  set pushRevision(value) {
    this.#pushRevisionSignal.set(value);
  }
  readonly #occurrencesSignal = signal<NotificationOccurrenceItem[]>([]);
  get occurrences(): NotificationOccurrenceItem[] {
    return this.#occurrencesSignal.get();
  }
  set occurrences(value: NotificationOccurrenceItem[]) {
    this.#occurrencesSignal.set(value);
  }
  /** Raw server rows consumed by the retained occurrence page. */
  readonly #consumedCountSignal = signal(0);
  get consumedCount() {
    return this.#consumedCountSignal.get();
  }
  set consumedCount(value) {
    this.#consumedCountSignal.set(value);
  }
  /** Exact visible occurrence total reported with the retained page. */
  readonly #totalCountSignal = signal(0);
  get totalCount() {
    return this.#totalCountSignal.get();
  }
  set totalCount(value) {
    this.#totalCountSignal.set(value);
  }
  /** Whether the retained page has older server rows available. */
  readonly #hasMoreSignal = signal(false);
  get hasMore() {
    return this.#hasMoreSignal.get();
  }
  set hasMore(value) {
    this.#hasMoreSignal.set(value);
  }
  readonly #unreadNotificationCountSignal = signal(0);
  get unreadNotificationCount() {
    return this.#unreadNotificationCountSignal.get();
  }
  set unreadNotificationCount(value) {
    this.#unreadNotificationCountSignal.set(value);
  }
  readonly #importantUnreadNotificationCountSignal = signal(0);
  get importantUnreadNotificationCount() {
    return this.#importantUnreadNotificationCountSignal.get();
  }
  set importantUnreadNotificationCount(value) {
    this.#importantUnreadNotificationCountSignal.set(value);
  }
  readonly #roomUnreadCountsSignal = signal<Record<string, number>>({});
  get roomUnreadCounts(): Record<string, number> {
    return this.#roomUnreadCountsSignal.get();
  }
  set roomUnreadCounts(value: Record<string, number>) {
    this.#roomUnreadCountsSignal.set(value);
  }
  readonly #roomImportantUnreadCountsSignal = signal<Record<string, number>>({});
  get roomImportantUnreadCounts(): Record<string, number> {
    return this.#roomImportantUnreadCountsSignal.get();
  }
  set roomImportantUnreadCounts(value: Record<string, number>) {
    this.#roomImportantUnreadCountsSignal.set(value);
  }
  readonly #nextExpiryAtSignal = signal<string | null>(null);
  get nextExpiryAt(): string | null {
    return this.#nextExpiryAtSignal.get();
  }
  set nextExpiryAt(value: string | null) {
    this.#nextExpiryAtSignal.set(value);
  }
  readonly revokedRoomIds = new ReactiveSet<string>();
  readonly #roomInvitations = new ReactiveMap<string, string>();

  /** Pending invitations expose only their exact occurrence, never room messages. */
  canDisplayOccurrence(occurrence: NotificationOccurrenceItem): boolean {
    const roomId = occurrence.room?.id;
    return (
      !roomId ||
      !this.revokedRoomIds.has(roomId) ||
      (occurrence.signalKind === NotificationSignalKind.ROOM_INVITATION &&
        !!occurrence.invitationId &&
        this.#roomInvitations.get(roomId) === occurrence.invitationId)
    );
  }

  /** Keep server pagination/counts while removing revoked room content. */
  #privacySafePage(
    page: NotificationOccurrencePage,
    authoritative = true
  ): NotificationOccurrencePage {
    const roomUnreadCounts = { ...page.roomUnreadCounts };
    const roomImportantUnreadCounts = { ...page.roomImportantUnreadCounts };
    let removedUnread = 0;
    let removedImportant = 0;
    for (const roomId of this.revokedRoomIds) {
      // A recipient has at most one pending invitation per room. Server counts
      // can include it even when its occurrence is on a later page.
      const invitationId = this.#roomInvitations.get(roomId);
      const matchesInvitation = (row: NotificationOccurrenceItem) =>
        row.room?.id === roomId &&
        row.signalKind === NotificationSignalKind.ROOM_INVITATION &&
        row.invitationId === invitationId;
      const knownInvitation =
        authoritative && invitationId ? page.occurrences.find(matchesInvitation) : undefined;
      // Only this server page can confirm a read. Retained rows can be
      // optimistic; off-page invitations use the authoritative aggregate.
      const limit = invitationId && knownInvitation?.unread !== false ? 1 : 0;
      const unread = Math.min(limit, roomUnreadCounts[roomId] ?? 0);
      const important = Math.min(unread, roomImportantUnreadCounts[roomId] ?? 0);
      removedUnread += (roomUnreadCounts[roomId] ?? 0) - unread;
      removedImportant += (roomImportantUnreadCounts[roomId] ?? 0) - important;
      if (unread) roomUnreadCounts[roomId] = unread;
      else delete roomUnreadCounts[roomId];
      if (important) roomImportantUnreadCounts[roomId] = important;
      else delete roomImportantUnreadCounts[roomId];
    }
    return {
      ...page,
      consumedCount: page.consumedCount ?? page.occurrences.length,
      occurrences: page.occurrences
        .filter((row) => this.canDisplayOccurrence(row))
        .map((row) => this.#privacySafeOccurrence(row)),
      unreadCount: Math.max(0, page.unreadCount - removedUnread),
      importantUnreadCount: Math.max(
        0,
        Math.min(page.importantUnreadCount - removedImportant, page.unreadCount - removedUnread)
      ),
      roomUnreadCounts,
      roomImportantUnreadCounts
    };
  }
  /** Users whose copied profile data must not be rendered from stale notification pages. */
  readonly scrubbedUserIds = new ReactiveSet<string>();
  readonly #loadingSignal = signal(false);
  get loading() {
    return this.#loadingSignal.get();
  }
  set loading(value) {
    this.#loadingSignal.set(value);
  }
  readonly #hasLoadedSignal = signal(false);
  get hasLoaded() {
    return this.#hasLoadedSignal.get();
  }
  set hasLoaded(value) {
    this.#hasLoadedSignal.set(value);
  }
  readonly #errorSignal = signal<unknown>(null);
  /** The error of the latest failed load, or null. */
  get error(): unknown {
    return this.#errorSignal.get();
  }
  set error(value: unknown) {
    this.#errorSignal.set(value);
  }

  constructor(api: NotificationAPI) {
    this.#api = api;
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
    this.#replaceSafeOccurrenceProjection(this.#privacySafePage(page));
  }

  /** Install a page whose room access and actor data have already been checked. */
  #replaceSafeOccurrenceProjection(page: NotificationOccurrencePage): void {
    this.#lastSuccessfulReadAt = Date.now();
    this.pushRevision++;
    this.#fetchGeneration++;
    this.#authoritativeGeneration++;
    this.#pendingDeletionById.clear();
    this.#pendingReadById.clear();
    this.#pendingReadRequestById.clear();
    this.occurrences = page.occurrences;
    this.consumedCount = page.consumedCount ?? page.occurrences.length;
    this.totalCount = page.totalCount;
    this.hasMore = page.hasMore && this.consumedCount > 0;
    this.unreadNotificationCount = page.unreadCount;
    this.importantUnreadNotificationCount = page.importantUnreadCount;
    this.roomUnreadCounts = page.roomUnreadCounts;
    this.roomImportantUnreadCounts = page.roomImportantUnreadCounts;
    this.nextExpiryAt = page.nextExpiryAt ?? null;
    this.loading = false;
    this.hasLoaded = true;
    this.error = null;
  }

  /** Invalidate projection-owned state while a snapshot hydrates. */
  resetProjectionState(): void {
    this.#reconciliation = undefined;
    this.#lastSuccessfulReadAt = null;
    this.pushRevision++;
    this.#handledPushIds.clear();
    this.#roomInvitations.clear();
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
    // a later resource read does not return a row.
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

  /** Revoke message notifications, retaining only an explicit pending invitation.
   * Returns true when a new invitation grant requires a fresh notification read.
   */
  clearRoom(roomId: string, invitationId?: string): boolean {
    const invitationAdded = !!invitationId && this.#roomInvitations.get(roomId) !== invitationId;
    this.#authoritativeGeneration++;
    if (invitationId) this.#roomInvitations.set(roomId, invitationId);
    else this.#roomInvitations.delete(roomId);
    this.revokedRoomIds.add(roomId);
    const removed = this.occurrences.filter((row) => !this.canDisplayOccurrence(row));
    for (const occurrence of removed) {
      this.#pendingDeletionById.delete(occurrence.id);
      this.#pendingReadById.delete(occurrence.id);
      this.#pendingReadRequestById.delete(occurrence.id);
    }
    const safe = this.#privacySafePage(
      {
        occurrences: this.occurrences,
        unreadCount: this.unreadNotificationCount,
        importantUnreadCount: this.importantUnreadNotificationCount,
        roomUnreadCounts: this.roomUnreadCounts,
        roomImportantUnreadCounts: this.roomImportantUnreadCounts,
        totalCount: this.totalCount,
        hasMore: this.hasMore
      },
      false
    );
    this.occurrences = safe.occurrences;
    this.consumedCount = this.occurrences.length;
    this.totalCount = Math.max(this.occurrences.length, this.totalCount - removed.length);
    this.unreadNotificationCount = safe.unreadCount;
    this.importantUnreadNotificationCount = safe.importantUnreadCount;
    this.roomUnreadCounts = safe.roomUnreadCounts;
    this.roomImportantUnreadCounts = safe.roomImportantUnreadCounts;
    this.nextExpiryAt = earliestNotificationOccurrenceExpiry(this.occurrences);
    return invitationAdded;
  }

  /** Re-open message notifications only after an explicit membership projection. */
  restoreRoom(roomId: string): void {
    this.#roomInvitations.delete(roomId);
    if (!this.revokedRoomIds.delete(roomId)) return;
    this.#authoritativeGeneration++;
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

  /**
   * Quietly recover missed hints. A host may reuse a successful first-page read
   * younger than maxAgeMs; failures and resets never postpone recovery.
   * Concurrent fallback calls share one read. The default always checks the server.
   */
  async reconcile(maxAgeMs = 0): Promise<void> {
    if (this.#reconciliation) return this.#reconciliation;
    if (
      !this.error &&
      this.#lastSuccessfulReadAt !== null &&
      Date.now() - this.#lastSuccessfulReadAt < maxAgeMs
    )
      return;
    const request = this.#refresh(false);
    this.#reconciliation = request;
    try {
      await request;
    } finally {
      if (this.#reconciliation === request) this.#reconciliation = undefined;
    }
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
      this.error = e;
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
    const safePage = this.#privacySafePage(page);
    if (offset === 0) this.#replaceSafeOccurrenceProjection(safePage);
    else {
      this.occurrences = mergeNotificationOccurrences(this.occurrences, safePage.occurrences);
      const consumedCount = safePage.consumedCount ?? safePage.occurrences.length;
      this.consumedCount = Math.max(this.consumedCount, offset + consumedCount);
      this.totalCount = safePage.totalCount;
      this.hasMore = safePage.hasMore && consumedCount > 0;
    }
    return safePage;
  }

  /** Load the remaining pages before an action selects all read occurrences. */
  async fetchAllPages(): Promise<void> {
    while (this.hasMore) {
      const offset = this.consumedCount;
      const generation = this.#authoritativeGeneration;
      await this.fetchPage(offset);
      if (offset > 0 && generation !== this.#authoritativeGeneration) {
        // A replacement can discard intermediate pages. Start again instead
        // of treating the retried append's offset as a complete prefix.
        await this.fetchPage(0);
        continue;
      }
      if (this.hasMore && this.consumedCount <= offset) {
        throw new Error('Notification pagination did not advance');
      }
    }
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
    const uniqueIds = [...new Set(notificationIds)];
    const removedIds = new Set(uniqueIds);
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
      this.#rememberHandledPushIds(uniqueIds);
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
      if (this.revokedRoomIds.has(roomId) && !this.#roomInvitations.has(roomId)) {
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
        if (this.revokedRoomIds.has(roomId) && !this.#roomInvitations.has(roomId)) {
          return { ok: true, totalCount: 0, notification: null };
        }
        const matches = page.occurrences.filter(
          (occurrence) =>
            occurrence.unread &&
            this.canDisplayOccurrence(occurrence) &&
            matchesRoomNotification(occurrence, roomId, options)
        );
        totalCount += matches.length;
        if (!matchedOccurrence && matches.length > 0) matchedOccurrence = matches[0]!;
        hasMore = page.hasMore;
        const consumedCount = page.consumedCount ?? page.occurrences.length;
        if (!hasMore || consumedCount === 0) break;
        offset += consumedCount;
      } while (hasMore);
      if (this.revokedRoomIds.has(roomId) && !this.#roomInvitations.has(roomId)) {
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
      this.error = e;
      console.error('Failed to fetch room notification:', e);
      return { ok: false, totalCount: null, notification: null };
    }
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
        this.#rememberHandledPushIds([notificationId]);
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
      : new Map<string, { unread: number; importantUnread: number }>();
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
      this.#rememberHandledPushIds([notificationId]);
      return true;
    } catch (e) {
      console.error('Failed to mark notification read:', e);
    } finally {
      if (this.#pendingReadById.get(notificationId) === mutation) {
        this.#pendingReadById.delete(notificationId);
      }
      if (request && this.#pendingReadRequestById.get(notificationId) === request) {
        this.#pendingReadRequestById.delete(notificationId);
      }
      this.#endMutation();
    }
    // Only the catch path reaches this point; success returns inside `try`.
    await this.#reconcileAfterFailedMutation();
    return false;
  }

  #beginMutation(): void {
    this.#reconciliation = undefined;
    this.#lastSuccessfulReadAt = null;
    this.pushRevision++;
    this.#pendingMutationCount++;
    this.#authoritativeGeneration++;
  }

  #endMutation(): void {
    this.pushRevision++;
    this.#pendingMutationCount = Math.max(0, this.#pendingMutationCount - 1);
    if (this.#pendingMutationCount !== 0) return;
    for (const resolve of this.#mutationIdleWaiters) resolve();
    this.#mutationIdleWaiters.clear();
  }

  async #waitForPendingMutations(): Promise<void> {
    if (this.#pendingMutationCount === 0) return;
    await new Promise<void>((resolve) => this.#mutationIdleWaiters.add(resolve));
  }

  /**
   * Check IDs captured from the OS before this read. Never use optimistic rows
   * or infer deletion from a partial page. Unknown older IDs remain displayed.
   * The caller must discard the result if pushRevision or account identity changes.
   */
  async handledPushNotificationIds(ids: readonly string[]): Promise<Set<string>> {
    if (this.#pendingMutationCount > 0) return new Set();
    const generation = this.#authoritativeGeneration;
    const handled = new Set(ids.filter((id) => this.#handledPushIds.has(id)));
    if (handled.size === ids.length) return handled;
    let page: NotificationOccurrencePage;
    try {
      page = await this.#api.listNotificationOccurrences(50);
    } catch {
      // A failed read must not block an already-confirmed local action.
      return handled;
    }
    if (generation !== this.#authoritativeGeneration || this.#pendingMutationCount > 0) {
      return new Set();
    }
    return handledIdsFromPage(ids, page, handled);
  }

  /**
   * Return confirmed handled IDs from retained state without a request. Missing
   * IDs on partial pages remain unknown. Mutations invalidate page evidence until
   * another authoritative read; confirmed local read/delete IDs remain usable.
   */
  knownHandledNotificationIds(ids: readonly string[]): Set<string> {
    const handled = new Set(ids.filter((id) => this.#handledPushIds.has(id)));
    if (this.#pendingMutationCount > 0 || this.#lastSuccessfulReadAt === null) return handled;
    return handledIdsFromPage(
      ids,
      {
        occurrences: this.occurrences,
        hasMore: this.hasMore,
        unreadCount: this.unreadNotificationCount
      },
      handled
    );
  }

  #rememberHandledPushIds(ids: readonly string[]): void {
    for (const id of ids) this.#handledPushIds.add(id);
    // This is only a local fast path. Fresh server reads recover after reload.
    while (this.#handledPushIds.size > 1024) {
      this.#handledPushIds.delete(this.#handledPushIds.values().next().value!);
    }
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
    return new Map([
      [
        knownCounts.roomId,
        { unread: knownCounts.unread, importantUnread: knownCounts.importantUnread }
      ]
    ]);
  }
  const result = new Map<string, { unread: number; importantUnread: number }>();
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
  const byId = new Map(current.map((occurrence) => [occurrence.id, occurrence]));
  for (const occurrence of restored) byId.set(occurrence.id, occurrence);
  return [...byId.values()].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}
