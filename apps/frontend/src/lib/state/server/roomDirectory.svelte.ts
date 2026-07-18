import { SvelteSet } from 'svelte/reactivity';
import type { DirectoryRoomSummary, RoomDirectoryAPI } from '$lib/api-client/roomDirectory';
import { RoomDirectoryScope, RoomKind } from '$lib/api-client/roomDirectory';
import type { MemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import type { UserAvatarUserView } from '$lib/render/types';
import { avatarUserFromDirectoryMember } from './rooms.svelte';

export type DirectoryRoom = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  isUniversal: boolean;
  viewerCanJoinRoom: boolean;
};

export type DirectoryRoomJoinPreview = {
  memberCount: number;
  sampleMembers: UserAvatarUserView[];
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
 * The page-level component triggers {@link refresh} on mount / server switch
 * and surfaces command feedback.
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
    private readonly memberDirectoryAPI: Pick<MemberDirectoryAPI, 'listRoomMembers'>,
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

  /** Replace the visible channel directory from the realtime projection. */
  replaceProjection(rooms: DirectoryRoomSummary[]): void {
    this.loadId++;
    this.allRooms = rooms.filter((room) => room.kind === RoomKind.CHANNEL).map(directoryRoom);
    this.justJoinedIds.clear();
    this.justLeftIds.clear();
    this.isLoading = false;
  }

  /** Invalidate projection-owned directory state during a compacted reset. */
  resetProjectionState(): void {
    this.loadId++;
    this.allRooms = [];
    this.justJoinedIds.clear();
    this.justLeftIds.clear();
    this.isLoading = true;
  }

  /**
   * Loads the first five alphabetically sorted room members and the exact
   * total from the ordinary paginated member listing. Availability remains
   * best-effort so a failed read never prevents the user from joining.
   */
  async loadJoinPreview(roomId: string): Promise<DirectoryRoomJoinPreview | null> {
    try {
      const page = await this.memberDirectoryAPI.listRoomMembers(roomId, '', 5, 0);
      return {
        memberCount: page.totalCount,
        sampleMembers: page.members.map(avatarUserFromDirectoryMember)
      };
    } catch {
      return null;
    }
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
}
