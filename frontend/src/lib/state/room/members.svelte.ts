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
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
};

export type RoomMembersState = {
  members: RoomMember[];
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
      livePresence: new SvelteMap(),
      presenceVersion: 0
    }
  });
  setMembersState(state);

  return {
    /** Replace the member list */
    setMembers(members: RoomMember[]) {
      state.current.members = members;
    },

    /** Update presence for a single user */
    updatePresence(userId: string, status: PresenceStatus) {
      state.current.livePresence.set(userId, status);
      state.current.presenceVersion++;
    },

    /** Clear all data (call when leaving room) */
    clear() {
      state.current.members = [];
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
