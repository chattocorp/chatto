import { SvelteSet } from 'svelte/reactivity';
import type { GetRoomDirectoryResponse } from '$lib/pb/chatto/api/v1/chat_pb';
import { tryWireJoinGroup, tryWireJoinRoom, tryWireLeaveRoom } from '$lib/wire';
import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
import { isRoomStateRefreshEvent } from './rooms.svelte';

export type DirectoryRoom = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  viewerCanJoinRoom: boolean;
};

export type JoinResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type LeaveResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type JoinGroupResult = { ok: true; joinedRoomIds: string[] } | { ok: false; error: Error };

export interface RoomDirectoryWireQueries {
  listRooms(): Promise<DirectoryRoom[] | null>;
}

export interface RoomDirectoryWireMutations {
  joinRoom(roomId: string): Promise<boolean>;
  leaveRoom(roomId: string): Promise<boolean>;
  joinGroup(groupId: string): Promise<string[] | null>;
}

const defaultWireMutations: RoomDirectoryWireMutations = {
  joinRoom: (roomId) => tryWireJoinRoom({ roomId }),
  leaveRoom: (roomId) => tryWireLeaveRoom({ roomId }),
  joinGroup: (groupId) => tryWireJoinGroup({ groupId })
};

function defaultWireQueries(serverId: string): RoomDirectoryWireQueries {
  return {
    async listRooms() {
      const client = wireEventBusManager.getClient(serverId);
      if (!client) return null;
      return directoryRoomsFromWire(await client.getRoomDirectory());
    }
  };
}

function mutationError(error: unknown, fallback: string): Error {
  return error instanceof Error ? error : new Error(fallback);
}

/**
 * Reactive state for the Browse Rooms directory page.
 *
 * Owns the "all rooms" listing (joined or not) plus the optimistic UI state
 * for in-flight join/leave operations (`joiningIds` / `leavingIds`) and the
 * just-completed momentary state (`justJoinedIds` / `justLeftIds`). The
 * actual "which rooms have I joined" answer comes from the active server's
 * rooms store — components combine the two via
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
    serverId: string,
    private readonly wireMutations: RoomDirectoryWireMutations = defaultWireMutations,
    private readonly wireQueries: RoomDirectoryWireQueries = defaultWireQueries(serverId)
  ) {}

  // ---------------------------------------------------------------------------
  // Loading
  // ---------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const rooms = await this.wireQueries.listRooms();
    if (this.loadId !== thisLoad) return;

    if (rooms) {
      this.allRooms = rooms;
      // A successful refresh confirms what was optimistically applied; clear
      // the just-* sets so isJoined() falls back on the authoritative joined
      // membership reported by RoomsStore.
      this.justJoinedIds.clear();
      this.justLeftIds.clear();
    }
    this.isLoading = false;
  }

  // ---------------------------------------------------------------------------
  // Membership predicate
  // ---------------------------------------------------------------------------

  /**
   * Whether a room should render as "joined" in the directory UI. Combines
   * authoritative membership (from `RoomsStore.rooms`, supplied by the
   * caller) with optimistic just-* state held here.
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
      const handledByWire = await this.wireMutations.joinRoom(roomId);
      if (!handledByWire) {
        return { ok: false, error: new Error('Failed to join room') };
      }

      this.justJoinedIds.add(roomId);
      this.justLeftIds.delete(roomId);
      return { ok: true, room: this.allRooms.find((r) => r.id === roomId) };
    } catch (error) {
      return { ok: false, error: mutationError(error, 'Failed to join room') };
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
      const joined = await this.wireMutations.joinGroup(groupId);
      if (!joined) {
        return { ok: false, error: new Error('Failed to join rooms') };
      }

      for (const id of joined) {
        this.justJoinedIds.add(id);
        this.justLeftIds.delete(id);
      }
      return { ok: true, joinedRoomIds: joined };
    } catch (error) {
      return { ok: false, error: mutationError(error, 'Failed to join rooms') };
    } finally {
      this.joiningGroupIds.delete(groupId);
    }
  }

  async leaveRoom(roomId: string): Promise<LeaveResult> {
    this.leavingIds.add(roomId);
    try {
      const handledByWire = await this.wireMutations.leaveRoom(roomId);
      if (!handledByWire) {
        return { ok: false, error: new Error('Failed to leave room') };
      }

      this.justLeftIds.add(roomId);
      this.justJoinedIds.delete(roomId);
      return { ok: true, room: this.allRooms.find((r) => r.id === roomId) };
    } catch (error) {
      return { ok: false, error: mutationError(error, 'Failed to leave room') };
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
   * minimal stub without needing to materialise a full RoomEventViewFragment
   * (the only field we touch is `event.__typename`).
   */
  ingestServerEvent(serverEvent: { event?: { __typename?: string } | null }): void {
    const event = serverEvent.event;
    if (!event) return;
    if (isRoomStateRefreshEvent(event.__typename)) {
      void this.refresh();
    }
  }

  /** Refresh when the room layout changes (admin reorders sections). */
  ingestRoomLayoutUpdated(): void {
    void this.refresh();
  }
}

function directoryRoomsFromWire(response: GetRoomDirectoryResponse): DirectoryRoom[] {
  const rooms: DirectoryRoom[] = [];
  for (const view of response.roomViews) {
    const room = view.room;
    if (!room) continue;
    rooms.push({
      id: room.id,
      name: room.name,
      description: room.description || null,
      archived: room.archived,
      viewerCanJoinRoom: view.viewerCanJoinRoom
    });
  }
  return rooms;
}
