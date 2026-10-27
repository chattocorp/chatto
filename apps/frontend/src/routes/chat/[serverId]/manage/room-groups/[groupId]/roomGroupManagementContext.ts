import { createContext } from 'svelte';
import type { AdminRoomGroup } from '$lib/api/adminRoomLayout';
import type { buildRoomGroupSettingsUpdate } from './roomGroupSettings';

/** Live layout state. Children mount only with access and a loaded group. */
export interface RoomGroupManagementContext {
  readonly groupId: string;
  readonly group: AdminRoomGroup;
  readonly canManageGroup: boolean;
  readonly saving: boolean;
  /** Changes after an accepted save to reset the draft baseline. */
  readonly formRevision: number;
  /** Saves a sparse patch with session, route, and snapshot protection. */
  saveGeneralSettings(input: ReturnType<typeof buildRoomGroupSettingsUpdate>): void;
}
const [getRoomGroupManagement, setRoomGroupManagement] =
  createContext<RoomGroupManagementContext>();
/** Provides the layout's live state to its sections. */
export function provideRoomGroupManagement(context: RoomGroupManagementContext): void {
  setRoomGroupManagement(context);
}
/** Reads the enclosing room-group layout's state. */
export function useRoomGroupManagement(): RoomGroupManagementContext {
  return getRoomGroupManagement();
}
