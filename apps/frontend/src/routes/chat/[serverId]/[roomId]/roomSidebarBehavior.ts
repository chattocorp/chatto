import type { RoomSidebarPanel, RoomSidebarPanelState } from '$lib/storage/roomSidebarPanel';

export const CHANNEL_ROOM_SIDEBAR_PANELS: RoomSidebarPanel[] = [
  'members',
  'search',
  'files',
  'pins',
  'call'
];
export const DM_ROOM_SIDEBAR_PANELS: RoomSidebarPanel[] = ['search', 'files', 'call'];

export function canBanMembersFromRoomSidebar(
  isDM: boolean,
  roomCanBanMembers: boolean | null | undefined
): boolean {
  return !isDM && !!roomCanBanMembers;
}

export function roomSidebarPanelForRoom(
  isDM: boolean,
  panel: RoomSidebarPanelState,
  livekitEnabled = true,
  messageSearchEnabled = true,
  pinnedMessagesEnabled = true
): RoomSidebarPanelState {
  if (panel === null) return null;
  const panels = isDM ? DM_ROOM_SIDEBAR_PANELS : CHANNEL_ROOM_SIDEBAR_PANELS;
  if (!panels.includes(panel)) return null;
  if (panel === 'call' && !livekitEnabled) return null;
  if (panel === 'search' && !messageSearchEnabled) return null;
  if (panel === 'pins' && !pinnedMessagesEnabled) return null;
  return panel;
}

export function roomSidebarPanelsForRoom(
  isDM: boolean,
  livekitEnabled: boolean,
  messageSearchEnabled = true,
  pinnedMessagesEnabled = true
): RoomSidebarPanel[] {
  const panels = isDM ? DM_ROOM_SIDEBAR_PANELS : CHANNEL_ROOM_SIDEBAR_PANELS;
  return panels.filter(
    (panel) =>
      (livekitEnabled || panel !== 'call') &&
      (messageSearchEnabled || panel !== 'search') &&
      (pinnedMessagesEnabled || panel !== 'pins')
  );
}

export function visibleRoomSidebarPanel(
  isDesktop: boolean,
  desktopPanel: RoomSidebarPanelState,
  mobilePanel: RoomSidebarPanelState
): RoomSidebarPanelState {
  return isDesktop ? desktopPanel : mobilePanel;
}
