/**
 * Svelte context for room-scoped client state. The state itself lives in
 * `@chatto/client/room/*`; these helpers only make it available to a room's
 * component subtree.
 */

import { createContext } from 'svelte';
import { RoomMembersStore, type RoomMember } from '@chatto/client/room/members';
import type { MentionRole } from '@chatto/client/room/mentionRoles';
import type { RoomPermissions } from '@chatto/client/room/permissions';

const [getMembersStoreContext, setMembersStoreContext] = createContext<() => RoomMembersStore>();

export function setRoomMembersStore<T extends RoomMembersStore | (() => RoomMembersStore)>(
  store: T
): T {
  setMembersStoreContext(typeof store === 'function' ? store : () => store);
  return store;
}

/** Provide a standalone member store without a room or a server, for fixtures. */
export function createRoomMembers(): RoomMembersStore {
  return setRoomMembersStore(new RoomMembersStore(''));
}

export function getRoomMembersStore(): RoomMembersStore {
  return getMembersStoreContext()();
}

/** Capture context during initialization, then resolve the selected room later. */
export function useRoomMembersStore(): () => RoomMembersStore {
  return getMembersStoreContext();
}

export function getRoomMembers(): RoomMember[] {
  return getRoomMembersStore().members;
}

const [getRoomPermissionsState, setRoomPermissionsState] = createContext<{
  current: RoomPermissions;
}>();

/**
 * Creates and sets the room permissions context.
 * Accepts a getter that computes permissions reactively — no $effect needed.
 * Must be called synchronously during component initialization.
 */
export function createRoomPermissions(getPermissions: () => RoomPermissions): void {
  setRoomPermissionsState({
    get current() {
      return getPermissions();
    }
  });
}

/**
 * Gets the current room permissions from context.
 */
export function getRoomPermissions(): RoomPermissions {
  const state = getRoomPermissionsState();
  return state.current;
}

/** Capture the permissions context during initialization, then read it later. */
export function useRoomPermissions(): () => RoomPermissions {
  const state = getRoomPermissionsState();
  return () => state.current;
}

const [getMentionRolesState, setMentionRolesState] = createContext<() => MentionRole[]>();

export function createMentionRoles(getRoles: () => MentionRole[] = () => []) {
  setMentionRolesState(getRoles);
}

export function getMentionRoles(): MentionRole[] {
  return getMentionRolesState()();
}
