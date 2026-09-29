import { createContext } from 'svelte';
import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
import type { SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';

/** The session, room, and room snapshot that one room mutation targets. */
export type RoomMutationScope = SessionSnapshot & {
  roomId: string;
  /** The room snapshot generation when the mutation started. */
  snapshotGeneration: number;
};

/**
 * The room that the room management layout loads, shared with its section
 * pages.
 *
 * The layout owns the room query, the session guard, and the realtime
 * handling that invalidates the room snapshot. The section pages are rendered
 * only while the room is loaded and the viewer can manage its permissions.
 */
export interface RoomManagementContext {
  /** Room ID from the current route. */
  readonly roomId: string;
  readonly room: AdminManagedRoom;
  /** True when the viewer can change the room settings and members. */
  readonly canManageRoom: boolean;
  /** The scroll container of the page, for lists that load more at its edge. */
  readonly scrollContainer: HTMLDivElement | undefined;
  /** Captures the current session, room, and snapshot generation. */
  mutationScope(): RoomMutationScope;
  /** True while the session and the route still target the mutation's room. */
  isCurrentRoom(target: RoomMutationScope | undefined): target is RoomMutationScope;
  /** True when the room snapshot did not change since the mutation started. */
  canApplyRoomSnapshot(target: RoomMutationScope): boolean;
}

const [getRoomManagement, setRoomManagement] = createContext<RoomManagementContext>();

/** Provides the room management context to the section pages. */
export function provideRoomManagement(context: RoomManagementContext): void {
  setRoomManagement(context);
}

/** Returns the room management context provided by the room management layout. */
export function useRoomManagement(): RoomManagementContext {
  return getRoomManagement();
}
