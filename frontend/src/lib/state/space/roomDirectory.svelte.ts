import { createContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';
import type { RoomEventViewFragment } from '$lib/gql/graphql';

export type DirectoryRoom = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  viewerCanJoinRoom: boolean;
};

const RoomsInSpaceQuery = graphql(`
  query GetAllRoomsInSpace {
    server {
      rooms(type: CHANNEL) {
        id
        name
        description
        archived
        viewerCanJoinRoom
      }
    }
  }
`);

const JoinRoomFromDirectory = graphql(`
  mutation JoinRoomFromDirectory($input: JoinRoomInput!) {
    joinRoom(input: $input)
  }
`);

const LeaveRoomFromDirectory = graphql(`
  mutation LeaveRoomFromDirectoryStore($input: LeaveRoomInput!) {
    leaveRoom(input: $input)
  }
`);

export type JoinResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type LeaveResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };

/**
 * Reactive state for the Browse Rooms directory page.
 *
 * Owns the "all rooms in this space" listing (joined or not) plus the
 * optimistic UI state for in-flight join/leave operations
 * (`joiningIds` / `leavingIds`) and the just-completed momentary state
 * (`justJoinedIds` / `justLeftIds`). The actual "which rooms have I joined"
 * answer comes from the existing {@link SpaceRoomsStore} — components combine
 * the two via `isJoined(roomId, joinedSet)` rather than this store
 * duplicating that data.
 *
 * The page-level component is responsible for:
 * - Constructing the store with `client` + `spaceId`
 * - Forwarding space events via {@link ingestServerEvent}
 * - Forwarding room-layout events via {@link ingestRoomLayoutUpdated}
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

  private loadId = 0;

  constructor(
    private readonly client: Client,
    private readonly spaceId: string
  ) {
    void this.refresh();
  }

  // ---------------------------------------------------------------------------
  // Loading
  // ---------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const result = await this.client.query(RoomsInSpaceQuery, {}).toPromise();
    if (this.loadId !== thisLoad) return;

    if (result.data?.server) {
      this.allRooms = result.data.server.rooms;
      // A successful refresh confirms what was optimistically applied; clear
      // the just-* sets so isJoined() falls back on the authoritative joined
      // membership reported by SpaceRoomsStore.
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
   * authoritative membership (from `SpaceRoomsStore.rooms`, supplied by the
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
      const result = await this.client
        .mutation(JoinRoomFromDirectory, { input: { roomId } })
        .toPromise();

      if (result.error) {
        return { ok: false, error: new Error(result.error.message) };
      }

      this.justJoinedIds.add(roomId);
      this.justLeftIds.delete(roomId);
      return { ok: true, room: this.allRooms.find((r) => r.id === roomId) };
    } finally {
      this.joiningIds.delete(roomId);
    }
  }

  async leaveRoom(roomId: string): Promise<LeaveResult> {
    this.leavingIds.add(roomId);
    try {
      const result = await this.client
        .mutation(LeaveRoomFromDirectory, { input: { roomId } })
        .toPromise();

      if (result.error) {
        return { ok: false, error: new Error(result.error.message) };
      }

      this.justLeftIds.add(roomId);
      this.justJoinedIds.delete(roomId);
      return { ok: true, room: this.allRooms.find((r) => r.id === roomId) };
    } finally {
      this.leavingIds.delete(roomId);
    }
  }

  // ---------------------------------------------------------------------------
  // Subscription event ingestion
  // ---------------------------------------------------------------------------

  /**
   * Refresh on membership / archive changes. Other event types are no-ops.
   * Mirrors the trigger set used by {@link SpaceRoomsStore.ingestServerEvent}.
   *
   * Accepts a discriminated-union envelope so the test harness can pass a
   * minimal stub without needing to materialise a full RoomEventViewFragment
   * (the only field we touch is `event.__typename`).
   */
  ingestServerEvent(serverEvent: { event?: { __typename?: string } | null }): void {
    const event = serverEvent.event;
    if (!event) return;
    if (
      event.__typename === 'UserJoinedRoomEvent' ||
      event.__typename === 'UserLeftRoomEvent' ||
      event.__typename === 'RoomArchivedEvent' ||
      event.__typename === 'RoomUnarchivedEvent'
    ) {
      void this.refresh();
    }
  }

  /** Refresh when the room layout changes (admin reorders sections). */
  ingestRoomLayoutUpdated(): void {
    void this.refresh();
  }
}

export const [getRoomDirectoryStore, setRoomDirectoryStore] =
  createContext<RoomDirectoryStore>();
