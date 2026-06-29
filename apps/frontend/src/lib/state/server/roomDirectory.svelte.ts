import { SvelteSet } from 'svelte/reactivity';
import type { RoomDirectoryAPI, DirectoryRoomSummary } from '@chatto/api-client/roomDirectory';
import { RoomDirectoryScope } from '@chatto/api-client/roomDirectory';
import type { RoomCommandAPI } from '@chatto/api-client/rooms';
import type { RoomEventKindSource } from '$lib/render/eventKinds';
import { isRoomStateRefreshEvent } from './rooms.svelte';

export type DirectoryRoom = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  isUniversal: boolean;
  viewerCanJoinRoom: boolean;
};

export type JoinResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type LeaveResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type JoinGroupResult = { ok: true; joinedRoomIds: string[] } | { ok: false; error: Error };

function directoryRoom(room: DirectoryRoomSummary): DirectoryRoom {
  return {
    id: room.id,
    name: room.name,
    description: room.description,
    archived: room.archived,
    isUniversal: room.isUniversal,
    viewerCanJoinRoom: room.canJoinRoom
  };
}

/**
 * Reactive state for the Browse Rooms directory page.
 *
 * Owns the "all rooms" listing (joined or not) plus the optimistic UI state
 * for in-flight join/leave operations (`joiningIds` / `leavingIds`) and the
 * just-completed momentary state (`justJoinedIds` / `justLeftIds`). The
 * actual "which rooms have I joined" answer comes from membership-filtered
 * rows in the active server's rooms store — components combine the two via
 * `isJoined(roomId, joinedSet)` rather than this store duplicating that
 * data.
 *
 * One store per registered server, owned by `ServerStateStore`. The
 * Browse Rooms page reads the active server's store via
 * `serverRegistry.getStore(getServerId()).roomDirectory` and triggers
 * `refresh()` reactively when the active server changes.
 *
 * The page-level component is responsible for:
 * - Forwarding events to {@link ingestServerEvent} and
 *   {@link ingestRoomLayoutUpdated}
 * - Triggering {@link refresh} on mount / server switch
 * - Surfacing toast feedback from the {@link joinRoom} / {@link leaveRoom}
 *   results
 */
export class RoomDirectoryStore {
  allRooms = $state<DirectoryRoom[]>([]);
  isLoading = $state(true);

  // Optimistic UI sets. Public for templates to read; mutated only by methods
  // on this store.
  joiningIds = new SvelteSet<string>();
  leavingIds = new SvelteSet<string>();
  justJoinedIds = new SvelteSet<string>();
  justLeftIds = new SvelteSet<string>();
  // Group IDs whose "Join all" action is currently in flight.
  joiningGroupIds = new SvelteSet<string>();

  private loadId = 0;

  constructor(
    private readonly roomDirectoryAPI: Pick<RoomDirectoryAPI, 'listRooms'>,
    private readonly roomAPI: Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'>
  ) {}

  // ---------------------------------------------------------------------------
  // Loading
  // ---------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const rooms = await this.roomDirectoryAPI.listRooms(RoomDirectoryScope.CHANNELS);
    if (this.loadId !== thisLoad) return;

    this.allRooms = rooms.map(directoryRoom);
    // A successful refresh confirms what was optimistically applied; clear
    // the just-* sets so isJoined() falls back on the authoritative joined
    // membership reported by RoomsStore.
    this.justJoinedIds.clear();
    this.justLeftIds.clear();
    this.isLoading = false;
  }

  // ---------------------------------------------------------------------------
  // Membership predicate
  // ---------------------------------------------------------------------------

  /**
   * Whether a room should render as "joined" in the directory UI. Combines
   * authoritative membership IDs (from `RoomsStore.rooms` rows where
   * `viewerIsMember` is true, supplied by the caller) with optimistic just-*
   * state held here.
   */
  isJoined(roomId: string, joinedRoomIds: ReadonlySet<string>): boolean {
    if (this.justLeftIds.has(roomId)) return false;
    if (this.justJoinedIds.has(roomId)) return true;
    return joinedRoomIds.has(roomId);
  }

  // ---------------------------------------------------------------------------
  // Mutations
  // ---------------------------------------------------------------------------

  async joinRoom(roomId: string): Promise<JoinResult> {
    this.joiningIds.add(roomId);
    try {
      await this.roomAPI.joinRoom(roomId);
      this.justJoinedIds.add(roomId);
      this.justLeftIds.delete(roomId);
      return { ok: true, room: this.allRooms.find((r) => r.id === roomId) };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      this.joiningIds.delete(roomId);
    }
  }

  /**
   * Join every room in a group that the caller can self-join and hasn't
   * already joined. Returns the IDs of the rooms that were newly joined;
   * already-joined and non-joinable rooms are silently skipped server-side.
   */
  async joinGroup(groupId: string): Promise<JoinGroupResult> {
    this.joiningGroupIds.add(groupId);
    try {
      const joined = await this.roomAPI.joinGroup(groupId);
      for (const id of joined) {
        this.justJoinedIds.add(id);
        this.justLeftIds.delete(id);
      }
      return { ok: true, joinedRoomIds: joined };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      this.joiningGroupIds.delete(groupId);
    }
  }

  async leaveRoom(roomId: string): Promise<LeaveResult> {
    this.leavingIds.add(roomId);
    try {
      await this.roomAPI.leaveRoom(roomId);
      this.justLeftIds.add(roomId);
      this.justJoinedIds.delete(roomId);
      return { ok: true, room: this.allRooms.find((r) => r.id === roomId) };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      this.leavingIds.delete(roomId);
    }
  }

  // ---------------------------------------------------------------------------
  // Subscription event ingestion
  // ---------------------------------------------------------------------------

  /**
   * Refresh on membership, room catalog, and group layout changes. Other
   * event types are no-ops. Mirrors the trigger set used by
   * {@link RoomsStore.ingestServerEvent}.
   *
   * Accepts a discriminated-union envelope so the test harness can pass a
   * minimal stub without needing to materialise a full RoomEventView.
   */
  ingestServerEvent(serverEvent: { event?: RoomEventKindSource }): void {
    const event = serverEvent.event;
    if (!event) return;
    if (isRoomStateRefreshEvent(event)) {
      void this.refresh();
    }
  }

  /** Refresh when the room layout changes (admin reorders sections). */
  ingestRoomLayoutUpdated(): void {
    void this.refresh();
  }
}
