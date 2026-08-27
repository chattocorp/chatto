import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';
import type { ServerProjectionStore } from './projection.svelte';

export type OptimisticRoomReadHandle = {
  commit(): void;
  rollback(): void;
};

type ProjectedUnreadSource = Pick<ServerProjectionStore, 'rooms' | 'viewer'>;

/**
 * Optimistic overlays for projection-owned room unread state.
 *
 * Authoritative unread answers are read directly from `ServerProjectionStore`.
 * Local posting/reading actions can temporarily override one room until the
 * next room viewer-state operation acknowledges that command.
 */
export class RoomUnreadStore {
  private roomOverrides = new SvelteMap<string, boolean>();
  private optimisticReadRooms = new SvelteSet<string>();
  private optimisticReads = new OptimisticMutationRegistry();
  private roomRevisions = new SvelteMap<string, number>();
  private revision = 0;
  private serverHasUnknownUnreadOverride = $state<boolean | null>(null);

  constructor(private readonly getProjection?: () => ProjectedUnreadSource) {}

  private optimisticReadKey(roomId: string): string {
    return `room:${roomId}`;
  }

  private roomRevision(roomId: string): number {
    return this.roomRevisions.get(roomId) ?? 0;
  }

  private advanceRoomRevision(roomId: string): void {
    this.revision += 1;
    this.roomRevisions.set(roomId, this.revision);
  }

  private invalidateOptimisticRead(roomId: string): void {
    this.optimisticReads.clear(this.optimisticReadKey(roomId));
    this.optimisticReadRooms.delete(roomId);
  }

  setRoomUnread(roomId: string, unread: boolean): void {
    this.advanceRoomRevision(roomId);
    this.invalidateOptimisticRead(roomId);
    this.roomOverrides.set(roomId, unread);
  }

  beginOptimisticRead(roomId: string): OptimisticRoomReadHandle {
    const token = this.optimisticReads.createToken();
    const roomRevision = this.roomRevision(roomId);
    const key = this.optimisticReadKey(roomId);

    this.optimisticReads.mark(key, token);
    this.optimisticReadRooms.add(roomId);

    return {
      commit: () => {
        if (!this.optimisticReads.isCurrent(key, token)) return;
        if (this.roomRevision(roomId) !== roomRevision) return;
        this.advanceRoomRevision(roomId);
        this.roomOverrides.set(roomId, false);
        this.optimisticReads.clear(key);
        this.optimisticReadRooms.delete(roomId);
      },
      rollback: () => {
        if (!this.optimisticReads.isCurrent(key, token)) return;
        this.optimisticReads.clear(key);
        this.optimisticReadRooms.delete(roomId);
      }
    };
  }

  get hasAnyUnread(): boolean {
    const roomIds = new SvelteSet<string>(this.roomOverrides.keys());
    for (const roomId of this.getProjection?.().rooms.keys() ?? []) roomIds.add(roomId);
    for (const roomId of roomIds) if (this.roomIsUnread(roomId)) return true;
    if (this.serverHasUnknownUnreadOverride !== null) {
      return this.serverHasUnknownUnreadOverride;
    }
    const projection = this.getProjection?.();
    return projection?.rooms.size === 0
      ? (projection.viewer?.viewerState?.hasUnreadRooms ?? false)
      : false;
  }

  getFirstUnreadRoomId(): string | null {
    const roomIds = new SvelteSet<string>(this.roomOverrides.keys());
    for (const roomId of this.getProjection?.().rooms.keys() ?? []) roomIds.add(roomId);
    for (const roomId of roomIds) if (this.roomIsUnread(roomId)) return roomId;
    return null;
  }

  roomIsUnread(roomId: string): boolean {
    if (this.optimisticReadRooms.has(roomId)) return false;
    const override = this.roomOverrides.get(roomId);
    if (override !== undefined) return override;
    return this.getProjection?.().rooms.get(roomId)?.room?.viewerState?.hasUnread ?? false;
  }

  captureSnapshotRevision(): number {
    return this.revision;
  }

  initRooms(
    rooms: Array<{ id: string; hasUnread: boolean }>,
    serverHasUnknownUnread = false,
    snapshotRevision = this.captureSnapshotRevision()
  ): void {
    const snapshotRoomIds = new SvelteSet(rooms.map((room) => room.id));
    for (const roomId of this.roomOverrides.keys()) {
      if (!snapshotRoomIds.has(roomId) && this.roomRevision(roomId) <= snapshotRevision) {
        this.roomOverrides.delete(roomId);
      }
    }
    this.updateRooms(rooms, snapshotRevision);
    this.serverHasUnknownUnreadOverride = serverHasUnknownUnread;
  }

  updateRooms(
    rooms: Array<{ id: string; hasUnread: boolean }>,
    snapshotRevision = this.captureSnapshotRevision()
  ): void {
    for (const room of rooms) {
      if (this.roomRevision(room.id) > snapshotRevision) continue;
      this.roomOverrides.set(room.id, room.hasUnread);
    }
  }

  resolveUnknownUnread(): void {
    this.serverHasUnknownUnreadOverride = false;
  }

  setServerHasUnread(hasUnread: boolean): void {
    this.serverHasUnknownUnreadOverride = hasUnread;
    if (!hasUnread) {
      this.optimisticReads.clearAll();
      this.optimisticReadRooms.clear();
      this.roomOverrides.clear();
    }
  }

  /** Clear only local overlays confirmed by this projected unread value. */
  acknowledgeRoomProjection(roomId: string, hasUnread: boolean | undefined): void {
    if (hasUnread === undefined) return;
    if (hasUnread && this.optimisticReadRooms.has(roomId)) return;
    this.advanceRoomRevision(roomId);
    if (hasUnread === false) this.invalidateOptimisticRead(roomId);
    this.roomOverrides.delete(roomId);
  }

  /** Remove all local state when the projected room itself disappears. */
  removeRoomProjection(roomId: string): void {
    this.advanceRoomRevision(roomId);
    this.invalidateOptimisticRead(roomId);
    this.roomOverrides.delete(roomId);
  }

  /** A viewer operation supersedes the server-level fallback overlay. */
  acknowledgeViewerProjection(): void {
    this.serverHasUnknownUnreadOverride = null;
  }

  clear(): void {
    this.optimisticReads.clearAll();
    this.optimisticReadRooms.clear();
    this.roomRevisions.clear();
    this.revision = 0;
    this.roomOverrides.clear();
    this.serverHasUnknownUnreadOverride = null;
  }
}
