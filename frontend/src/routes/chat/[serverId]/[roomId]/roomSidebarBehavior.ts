import type { RoomSidebarPanel, RoomSidebarPanelState } from '$lib/storage/roomSidebarPanel';

export const DM_ROOM_SIDEBAR_PANELS: RoomSidebarPanel[] = ['files'];

export function canBanMembersFromRoomSidebar(
  isDM: boolean,
  roomCanBanMembers: boolean | null | undefined
): boolean {
  return !isDM && !!roomCanBanMembers;
}

export function roomSidebarPanelForRoom(
  isDM: boolean,
  panel: RoomSidebarPanelState
): RoomSidebarPanelState {
  if (!isDM) return panel;
  return DM_ROOM_SIDEBAR_PANELS.includes(panel as RoomSidebarPanel) ? panel : null;
}
