import { browser } from '$app/environment';
import type { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import type { Message } from '@chatto/api-types/api/v1/message_types_pb';
import type { RealtimeProjectionPinnedMessageChange } from '@chatto/api-types/realtime/v1/realtime_pb';
import { RealtimeProjectionPinnedMessageAction } from '@chatto/api-types/realtime/v1/realtime_pb';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { createPinnedMessagesAPI, type PinnedMessagesAPI } from '$lib/api-client/pinnedMessages';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { serverStorageKey } from '$lib/storage/serverStorage';

export const ROOM_PINS_PAGE_SIZE = 50;
const STATUS_RETRY_BASE_MS = 1_000;
const STATUS_RETRY_MAX_MS = 30_000;

export class RoomPinsStore {
  items = $state.raw<PinnedMessage[]>([]);
  totalCount = $state(0);
  hasMore = $state(false);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  error = $state(false);
  loadMoreError = $state(false);
  private readonly api: PinnedMessagesAPI;
  readonly roomId: string;
  private readonly seenStorageKey: string;
  private hydrated = false;
  private retainCount = 0;
  private requestEpoch = 0;
  private hydrationPromise: Promise<void> | null = null;
  private pinStatuses = new SvelteMap<string, boolean>();
  private pendingStatusIds = new SvelteSet<string>();
  private statusRequestScheduled = false;
  private statusRequestInFlight = false;
  private statusRetryTimer: ReturnType<typeof setTimeout> | null = null;
  private statusRetryAttempt = 0;
  private statusLookupsSuspended = false;
  private accessBlocked = false;
  private latestKnownMarker = $state('');
  private lastSeenMarker = $state('');

  constructor(serverConnection: ServerConnection, serverId: string, roomId: string) {
    this.roomId = roomId;
    this.api = serverConnection.getAPI(createPinnedMessagesAPI);
    this.seenStorageKey = serverStorageKey(serverId, `room:${roomId}:pinsSeen`);
    if (browser) this.lastSeenMarker = localStorage.getItem(this.seenStorageKey) ?? '';
  }

  get hasUnseen(): boolean {
    return this.latestKnownMarker !== '' && this.latestKnownMarker !== this.lastSeenMarker;
  }

  isPinned(messageEventId: string): boolean {
    return this.pinStatuses.get(messageEventId) ?? false;
  }

  hasPinStatus(messageEventId: string): boolean {
    return this.pinStatuses.has(messageEventId);
  }

  ensureStatus(messageEventId: string): void {
    if (
      !messageEventId ||
      this.accessBlocked ||
      this.statusLookupsSuspended ||
      this.pinStatuses.has(messageEventId)
    )
      return;
    this.pendingStatusIds.add(messageEventId);
    this.scheduleStatusFlush();
  }

  retain(): () => void {
    this.retainCount++;
    if (this.retainCount === 1) {
      this.statusLookupsSuspended = false;
      if (!this.accessBlocked) void this.hydrate();
    }
    let retained = true;
    return () => {
      if (!retained) return;
      retained = false;
      this.retainCount = Math.max(0, this.retainCount - 1);
      if (this.retainCount === 0) this.suspendStatusLookups();
    };
  }

  async hydrate(): Promise<void> {
    if (this.accessBlocked || this.hydrated || this.hydrationPromise)
      return this.hydrationPromise ?? undefined;
    const epoch = this.requestEpoch;
    this.hydrationPromise = this.loadPage(0, true, epoch);
    try {
      await this.hydrationPromise;
    } finally {
      if (this.requestEpoch === epoch) this.hydrationPromise = null;
    }
  }

  async loadMore(): Promise<void> {
    if (this.accessBlocked || !this.hydrated || this.isLoadingMore || !this.hasMore) return;
    const epoch = this.requestEpoch;
    this.isLoadingMore = true;
    try {
      await this.loadPage(this.items.length, false, epoch);
    } finally {
      if (this.requestEpoch === epoch) this.isLoadingMore = false;
    }
  }

  async create(messageEventId: string): Promise<void> {
    if (this.accessBlocked) return;
    const item = await this.api.create(this.roomId, messageEventId);
    if (!item) return;
    this.pinStatuses.set(messageEventId, true);
    this.invalidateAndReload();
  }

  async remove(messageEventId: string): Promise<void> {
    if (this.accessBlocked) return;
    await this.api.remove(this.roomId, messageEventId);
    this.removeLocal(messageEventId);
    this.invalidateAndReload();
  }

  applyRealtimeChange(change: RealtimeProjectionPinnedMessageChange, changeEventId: string): void {
    if (this.accessBlocked || change.roomId !== this.roomId) return;
    if (change.action === RealtimeProjectionPinnedMessageAction.CREATED) {
      this.pinStatuses.set(change.messageEventId, true);
      this.noteLatest(changeEventId);
      this.invalidateAndReload();
    } else if (change.action === RealtimeProjectionPinnedMessageAction.DELETED) {
      this.removeLocal(change.messageEventId);
      this.invalidateAndReload();
    }
  }

  applyMessageRetraction(messageEventId: string): void {
    if (this.accessBlocked) return;
    this.removeLocal(messageEventId);
    this.invalidateAndReload();
  }

  applyMessageUpdate(messageEventId: string, message: Message): void {
    if (this.accessBlocked || !this.isPinned(messageEventId)) return;
    this.items = this.items.map((item) => {
      if (item.message?.id !== messageEventId) return item;
      const updated = item.clone();
      updated.message = message;
      return updated;
    });
  }

  scrubUserReferences(userId: string): void {
    this.items = this.items.map((item) => {
      if (item.actor?.id !== userId && item.pinnedBy?.id !== userId) return item;
      const scrubbed = item.clone();
      if (scrubbed.actor?.id === userId) scrubbed.actor = undefined;
      if (scrubbed.pinnedBy?.id === userId) scrubbed.pinnedBy = undefined;
      return scrubbed;
    });
  }

  markSeen(): void {
    if (!this.latestKnownMarker) return;
    this.lastSeenMarker = this.latestKnownMarker;
    if (browser) localStorage.setItem(this.seenStorageKey, this.lastSeenMarker);
  }

  reset(options: { rehydrateRetained?: boolean; accessRevoked?: boolean } = {}): void {
    this.requestEpoch++;
    if (options.accessRevoked) this.accessBlocked = true;
    this.items = [];
    this.totalCount = 0;
    this.hasMore = false;
    this.isInitialLoading = true;
    this.error = false;
    this.loadMoreError = false;
    this.hydrated = false;
    this.hydrationPromise = null;
    this.pinStatuses.clear();
    this.pendingStatusIds.clear();
    this.statusRequestScheduled = false;
    if (this.statusRetryTimer) clearTimeout(this.statusRetryTimer);
    this.statusRetryTimer = null;
    this.statusRetryAttempt = 0;
    this.latestKnownMarker = '';
    if (options.rehydrateRetained && this.retainCount > 0 && !this.accessBlocked)
      void this.hydrate();
  }

  restoreAfterAccessGrant(): void {
    this.accessBlocked = false;
    if (this.retainCount > 0 && !this.hydrated) void this.hydrate();
  }

  dispose(): void {
    this.reset({ accessRevoked: true });
    this.retainCount = 0;
    this.statusLookupsSuspended = true;
  }

  retry(): void {
    this.invalidateAndReload();
  }

  private async loadPage(offset: number, replace: boolean, epoch: number): Promise<void> {
    if (replace) this.isInitialLoading = true;
    if (replace) this.error = false;
    else this.loadMoreError = false;
    try {
      const page = await this.api.list(this.roomId, ROOM_PINS_PAGE_SIZE, offset);
      if (this.requestEpoch !== epoch) return;
      this.items = replace ? page.items : [...this.items, ...page.items];
      for (const item of page.items) {
        if (item.message?.id) this.pinStatuses.set(item.message.id, true);
      }
      this.totalCount = page.totalCount;
      this.hasMore = page.hasMore;
      this.hydrated = true;
      if (replace) this.noteLatest(page.latestPinEventId);
    } catch {
      if (this.requestEpoch === epoch) {
        if (replace) this.error = true;
        else this.loadMoreError = true;
      }
    } finally {
      if (this.requestEpoch === epoch && replace) this.isInitialLoading = false;
    }
  }

  private noteLatest(marker: string): void {
    if (marker) this.latestKnownMarker = marker;
  }

  private removeLocal(messageEventId: string): void {
    this.pinStatuses.set(messageEventId, false);
    const next = this.items.filter((item) => item.message?.id !== messageEventId);
    if (next.length === this.items.length) return;
    this.items = next;
    this.totalCount = Math.max(0, this.totalCount - 1);
    this.hasMore = this.totalCount > this.items.length;
  }

  private invalidateAndReload(): void {
    this.requestEpoch++;
    this.hydrated = false;
    this.hydrationPromise = null;
    if (this.retainCount > 0) void this.hydrate();
  }

  private async flushPendingStatuses(): Promise<void> {
    this.statusRequestScheduled = false;
    if (this.accessBlocked || this.statusLookupsSuspended || this.statusRequestInFlight) return;
    const messageEventIds = [...this.pendingStatusIds];
    this.pendingStatusIds.clear();
    if (messageEventIds.length === 0) return;
    const epoch = this.requestEpoch;
    this.statusRequestInFlight = true;
    try {
      const batches: PinnedMessage[][] = [];
      for (let start = 0; start < messageEventIds.length; start += 100) {
        if (
          this.accessBlocked ||
          this.statusLookupsSuspended ||
          this.requestEpoch !== epoch
        ) {
          this.requeueStatuses(messageEventIds);
          return;
        }
        const batch = await this.api.batchGet(
          this.roomId,
          messageEventIds.slice(start, start + 100)
        );
        if (this.accessBlocked || this.statusLookupsSuspended) return;
        batches.push(batch);
      }
      if (this.requestEpoch !== epoch) {
        this.requeueStatuses(messageEventIds);
        return;
      }
      this.statusRetryAttempt = 0;
      for (const messageEventId of messageEventIds) this.pinStatuses.set(messageEventId, false);
      for (const item of batches.flat()) {
        if (item.message?.id) this.pinStatuses.set(item.message.id, true);
      }
    } catch {
      this.scheduleStatusRetry(messageEventIds);
    } finally {
      this.statusRequestInFlight = false;
      this.scheduleStatusFlush();
    }
  }

  private scheduleStatusRetry(messageEventIds: string[]): void {
    if (this.accessBlocked || this.statusLookupsSuspended) return;
    for (const messageEventId of messageEventIds) {
      if (!this.pinStatuses.has(messageEventId)) this.pendingStatusIds.add(messageEventId);
    }
    if (this.pendingStatusIds.size === 0 || this.statusRetryTimer) return;
    const delay = Math.min(
      STATUS_RETRY_BASE_MS * 2 ** this.statusRetryAttempt,
      STATUS_RETRY_MAX_MS
    );
    this.statusRetryAttempt++;
    this.statusRetryTimer = setTimeout(() => {
      this.statusRetryTimer = null;
      this.scheduleStatusFlush();
    }, delay);
  }

  private requeueStatuses(messageEventIds: string[]): void {
    if (this.accessBlocked || this.statusLookupsSuspended) return;
    for (const messageEventId of messageEventIds) {
      if (!this.pinStatuses.has(messageEventId)) this.pendingStatusIds.add(messageEventId);
    }
  }

  private scheduleStatusFlush(): void {
    if (
      this.accessBlocked ||
      this.statusLookupsSuspended ||
      this.statusRequestScheduled ||
      this.statusRequestInFlight ||
      this.statusRetryTimer ||
      this.pendingStatusIds.size === 0
    )
      return;
    this.statusRequestScheduled = true;
    queueMicrotask(() => void this.flushPendingStatuses());
  }

  private suspendStatusLookups(): void {
    this.statusLookupsSuspended = true;
    this.pendingStatusIds.clear();
    this.statusRequestScheduled = false;
    if (this.statusRetryTimer) clearTimeout(this.statusRetryTimer);
    this.statusRetryTimer = null;
    this.statusRetryAttempt = 0;
  }
}
