import { createContext } from 'svelte';

export type RoomPermissions = {
  canPostMessage: boolean;
  canPostInThread: boolean;
  canReact: boolean;
  canManageOthersMessage: boolean;
  canEchoMessage: boolean;
  canManageRoom: boolean;
  canBanRoomMembers: boolean;
};

export const DEFAULT_ROOM_PERMISSIONS: RoomPermissions = {
  canPostMessage: false,
  canPostInThread: false,
  canReact: false,
  canManageOthersMessage: false,
  canEchoMessage: false,
  canManageRoom: false,
  canBanRoomMembers: false
};

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
