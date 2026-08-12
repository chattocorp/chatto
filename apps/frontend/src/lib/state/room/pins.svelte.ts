import { browser } from '$app/environment';
import type { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import type { Message } from '@chatto/api-types/api/v1/message_types_pb';
import type { RealtimeProjectionPinnedMessageChange } from '@chatto/api-types/realtime/v1/realtime_pb';
import { RealtimeProjectionPinnedMessageAction } from '@chatto/api-types/realtime/v1/realtime_pb';
import { SvelteMap } from 'svelte/reactivity';
import { createPinnedMessagesAPI, type PinnedMessagesAPI } from '$lib/api-client/pinnedMessages';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { serverStorageKey } from '$lib/storage/serverStorage';

export const ROOM_PINS_PAGE_SIZE = 50;

export function roomPinsSeenStorageKey(serverId: string, viewerId: string, roomId: string): string {
  return serverStorageKey(serverId, `viewer:${viewerId}:room:${roomId}:pinsSeen`);
}

export function clearRoomPinsSeenMarker(serverId: string, viewerId: string, roomId: string): void {
  if (browser && viewerId)
    localStorage.removeItem(roomPinsSeenStorageKey(serverId, viewerId, roomId));
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
  private readonly serverId: string;
  private readonly viewerId: string;
  private readonly seenStorageKey: string;
  private hydrated = false;
  private retainCount = 0;
  private requestEpoch = 0;
  private hydrationPromise: Promise<void> | null = null;
  private pinStatuses = new SvelteMap<string, boolean>();
  private accessBlocked = false;
  private latestKnownMarker = $state('');
  private lastSeenMarker = $state('');

  constructor(
    serverConnection: ServerConnection,
    serverId: string,
    viewerId: string,
    roomId: string
  ) {
    this.roomId = roomId;
    this.serverId = serverId;
    this.viewerId = viewerId;
    this.api = serverConnection.getAPI(createPinnedMessagesAPI);
    this.seenStorageKey = roomPinsSeenStorageKey(serverId, viewerId, roomId);
    if (browser) this.lastSeenMarker = localStorage.getItem(this.seenStorageKey) ?? '';
  }

  get hasUnseen(): boolean {
    return (
      this.totalCount > 0 &&
      this.latestKnownMarker !== '' &&
      this.latestKnownMarker !== this.lastSeenMarker
    );
  }

  isPinned(messageEventId: string, hydratedStatus = false): boolean {
    return this.pinStatuses.get(messageEventId) ?? hydratedStatus;
  }

  retain(): () => void {
    this.retainCount++;
    if (this.retainCount === 1) {
      if (!this.accessBlocked) void this.hydrate();
    }
    let retained = true;
    return () => {
      if (!retained) return;
      retained = false;
      this.retainCount = Math.max(0, this.retainCount - 1);
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
    const epoch = this.requestEpoch;
    const item = await this.api.create(this.roomId, messageEventId);
    if (!item || this.accessBlocked || this.requestEpoch !== epoch) return;
    this.pinStatuses.set(messageEventId, true);
    this.invalidateAndReload();
  }

  async remove(messageEventId: string): Promise<void> {
    if (this.accessBlocked) return;
    const epoch = this.requestEpoch;
    await this.api.remove(this.roomId, messageEventId);
    if (this.accessBlocked || this.requestEpoch !== epoch) return;
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

  markSeen(): void {
    if (!this.latestKnownMarker) return;
    this.lastSeenMarker = this.latestKnownMarker;
    if (browser) localStorage.setItem(this.seenStorageKey, this.lastSeenMarker);
  }

  reset(options: { rehydrateRetained?: boolean; accessRevoked?: boolean } = {}): void {
    this.requestEpoch++;
    this.isLoadingMore = false;
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
    this.latestKnownMarker = '';
    if (options.accessRevoked) {
      this.lastSeenMarker = '';
      clearRoomPinsSeenMarker(this.serverId, this.viewerId, this.roomId);
    }
    if (options.rehydrateRetained && this.retainCount > 0 && !this.accessBlocked)
      void this.hydrate();
  }

  restoreAfterAccessGrant(): void {
    this.accessBlocked = false;
    if (this.retainCount > 0 && !this.hydrated) void this.hydrate();
  }

  dispose(): void {
    this.reset();
    this.accessBlocked = true;
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
      if (replace) this.noteLatest(page.latestPinMarker);
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
    this.isLoadingMore = false;
    this.hydrated = false;
    this.hydrationPromise = null;
    if (this.retainCount > 0) void this.hydrate();
  }
}
