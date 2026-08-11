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

function pinStamp(item: Pick<PinnedMessage, 'pinnedAt'>): string {
  const at = item.pinnedAt;
  return at
    ? `${at.seconds.toString().padStart(20, '0')}:${at.nanos.toString().padStart(9, '0')}`
    : '';
}

function changeStamp(change: RealtimeProjectionPinnedMessageChange): string {
  const at = change.pinnedAt;
  return at
    ? `${at.seconds.toString().padStart(20, '0')}:${at.nanos.toString().padStart(9, '0')}`
    : '';
}

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
  private latestKnownStamp = $state('');
  private lastSeenStamp = $state('');

  constructor(serverConnection: ServerConnection, serverId: string, roomId: string) {
    this.roomId = roomId;
    this.api = serverConnection.getAPI(createPinnedMessagesAPI);
    this.seenStorageKey = serverStorageKey(serverId, `room:${roomId}:pinsSeenAt`);
    if (browser) this.lastSeenStamp = localStorage.getItem(this.seenStorageKey) ?? '';
  }

  get hasUnseen(): boolean {
    return this.latestKnownStamp > this.lastSeenStamp;
  }

  isPinned(messageEventId: string): boolean {
    return this.pinStatuses.get(messageEventId) ?? false;
  }

  hasPinStatus(messageEventId: string): boolean {
    return this.pinStatuses.has(messageEventId);
  }

  ensureStatus(messageEventId: string): void {
    if (!messageEventId || this.pinStatuses.has(messageEventId)) return;
    this.pendingStatusIds.add(messageEventId);
    if (this.statusRequestScheduled) return;
    this.statusRequestScheduled = true;
    queueMicrotask(() => void this.flushPendingStatuses());
  }

  retain(): () => void {
    this.retainCount++;
    if (this.retainCount === 1) void this.hydrate();
    let retained = true;
    return () => {
      if (!retained) return;
      retained = false;
      this.retainCount = Math.max(0, this.retainCount - 1);
    };
  }

  async hydrate(): Promise<void> {
    if (this.hydrated || this.hydrationPromise) return this.hydrationPromise ?? undefined;
    const epoch = this.requestEpoch;
    this.hydrationPromise = this.loadPage(0, true, epoch);
    try {
      await this.hydrationPromise;
    } finally {
      if (this.requestEpoch === epoch) this.hydrationPromise = null;
    }
  }

  async loadMore(): Promise<void> {
    if (!this.hydrated || this.isLoadingMore || !this.hasMore) return;
    const epoch = this.requestEpoch;
    this.isLoadingMore = true;
    try {
      await this.loadPage(this.items.length, false, epoch);
    } finally {
      if (this.requestEpoch === epoch) this.isLoadingMore = false;
    }
  }

  async create(messageEventId: string): Promise<void> {
    const item = await this.api.create(this.roomId, messageEventId);
    if (!item) return;
    this.pinStatuses.set(messageEventId, true);
    this.noteLatest(pinStamp(item));
    this.invalidateAndReload();
  }

  async remove(messageEventId: string): Promise<void> {
    await this.api.remove(this.roomId, messageEventId);
    this.removeLocal(messageEventId);
    this.invalidateAndReload();
  }

  applyRealtimeChange(change: RealtimeProjectionPinnedMessageChange): void {
    if (change.roomId !== this.roomId) return;
    if (change.action === RealtimeProjectionPinnedMessageAction.CREATED) {
      this.pinStatuses.set(change.messageEventId, true);
      this.noteLatest(changeStamp(change));
      this.invalidateAndReload();
    } else if (change.action === RealtimeProjectionPinnedMessageAction.DELETED) {
      this.removeLocal(change.messageEventId);
      this.invalidateAndReload();
    }
  }

  applyMessageRetraction(messageEventId: string): void {
    this.removeLocal(messageEventId);
    this.invalidateAndReload();
  }

  applyMessageUpdate(messageEventId: string, message: Message): void {
    if (!this.isPinned(messageEventId)) return;
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
    if (!this.latestKnownStamp) return;
    this.lastSeenStamp = this.latestKnownStamp;
    if (browser) localStorage.setItem(this.seenStorageKey, this.lastSeenStamp);
  }

  reset(options: { rehydrateRetained?: boolean } = {}): void {
    this.requestEpoch++;
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
    this.latestKnownStamp = '';
    if (options.rehydrateRetained && this.retainCount > 0) void this.hydrate();
  }

  restoreAfterAccessGrant(): void {
    if (this.retainCount > 0 && !this.hydrated) void this.hydrate();
  }

  dispose(): void {
    this.reset();
    this.retainCount = 0;
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
      if (replace) this.latestKnownStamp = '';
      for (const item of page.items) this.noteLatest(pinStamp(item));
    } catch {
      if (this.requestEpoch === epoch) {
        if (replace) this.error = true;
        else this.loadMoreError = true;
      }
    } finally {
      if (this.requestEpoch === epoch && replace) this.isInitialLoading = false;
    }
  }

  private noteLatest(stamp: string): void {
    if (stamp > this.latestKnownStamp) this.latestKnownStamp = stamp;
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
    const messageEventIds = [...this.pendingStatusIds];
    this.pendingStatusIds.clear();
    if (messageEventIds.length === 0) return;
    const epoch = this.requestEpoch;
    try {
      const batches: PinnedMessage[][] = [];
      for (let start = 0; start < messageEventIds.length; start += 100) {
        batches.push(await this.api.batchGet(this.roomId, messageEventIds.slice(start, start + 100)));
      }
      if (this.requestEpoch !== epoch) {
        for (const messageEventId of messageEventIds) this.ensureStatus(messageEventId);
        return;
      }
      for (const messageEventId of messageEventIds) this.pinStatuses.set(messageEventId, false);
      for (const item of batches.flat()) {
        if (item.message?.id) this.pinStatuses.set(item.message.id, true);
      }
    } catch {
      // Leave these statuses unknown so remounting the message can retry.
    }
  }
}
