import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import type { PresenceStatus } from '$lib/gql/graphql';

/**
 * Room member data for the current room.
 * Set by Room.svelte, consumed by MessageComposer (autocomplete) and RoomSidebar (member list).
 *
 * Uses a reactive state object so the context can be set synchronously
 * during component initialization, then updated when data loads.
 */
export type RoomMember = {
  id: string;
  login: string;
  displayName: string;
  deleted?: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
};

export type RoomMembersState = {
  members: RoomMember[];
  totalCount: number;
  hasMore: boolean;
  loadingMore: boolean;
  /** Live presence updates - may contain more recent status than members array */
  livePresence: SvelteMap<string, PresenceStatus>;
  /**
   * Monotonically increasing counter, bumped on every presence update.
   * Reading this inside a $derived guarantees re-evaluation when any
   * presence value changes — unlike SvelteMap.size which only changes
   * when keys are added/removed, not when existing values change.
   */
  presenceVersion: number;
};

const [getMembersState, setMembersState] = createContext<{ current: RoomMembersState }>();

/**
 * Creates and sets the room members context.
 * Must be called synchronously during component initialization.
 * Returns an object with methods to update and interact with the store.
 */
export function createRoomMembers() {
  const state = $state<{ current: RoomMembersState }>({
    current: {
      members: [],
      totalCount: 0,
      hasMore: false,
      loadingMore: false,
      livePresence: new SvelteMap(),
      presenceVersion: 0
    }
  });
  setMembersState(state);

  return {
    get current() {
      return state.current;
    },

    /** Replace the member list */
    setMembers(
      members: RoomMember[],
      pagination: { totalCount?: number; hasMore?: boolean } = {}
    ) {
      state.current.members = members;
      state.current.totalCount = pagination.totalCount ?? members.length;
      state.current.hasMore = pagination.hasMore ?? false;
    },

    /** Append another page, skipping members already loaded */
    appendMembers(
      members: RoomMember[],
      pagination: { totalCount?: number; hasMore?: boolean } = {}
    ) {
      const nextMembers = members.filter(
        (member) => !state.current.members.some((loaded) => loaded.id === member.id)
      );
      state.current.members = [...state.current.members, ...nextMembers];
      state.current.totalCount = pagination.totalCount ?? state.current.members.length;
      state.current.hasMore = pagination.hasMore ?? false;
    },

    setLoadingMore(loading: boolean) {
      state.current.loadingMore = loading;
    },

    /** Update presence for a single user */
    updatePresence(userId: string, status: PresenceStatus) {
      state.current.livePresence.set(userId, status);
      state.current.presenceVersion++;
    },

    /** Clear all data (call when leaving room) */
    clear() {
      state.current.members = [];
      state.current.totalCount = 0;
      state.current.hasMore = false;
      state.current.loadingMore = false;
      state.current.livePresence.clear();
      state.current.presenceVersion = 0;
    }
  };
}

/**
 * Gets the room members state from context.
 * Returns the full state including live presence map.
 */
export function getRoomMembersState(): RoomMembersState {
  const state = getMembersState();
  return state.current;
}

/**
 * Gets just the member list (for simple use cases like autocomplete).
 */
export function getRoomMembers(): RoomMember[] {
  return getRoomMembersState().members;
}

/**
 * Gets the effective presence for a member (live update or fall back to initial value).
 */
export function getMemberPresence(member: RoomMember): PresenceStatus {
  const state = getRoomMembersState();
  return state.livePresence.get(member.id) ?? member.presenceStatus;
}
