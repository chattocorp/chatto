import { serverSlot, type Codec } from './slot';

export const ROOM_SIDEBAR_PANELS = ['members', 'files'] as const;

export type RoomSidebarPanel = (typeof ROOM_SIDEBAR_PANELS)[number];
export type RoomSidebarPanelState = RoomSidebarPanel | null;

export const ROOM_SIDEBAR_DEFAULT_PANEL: RoomSidebarPanel = 'members';

function isRoomSidebarPanel(value: unknown): value is RoomSidebarPanel {
  return typeof value === 'string' && ROOM_SIDEBAR_PANELS.includes(value as RoomSidebarPanel);
}

const codec: Codec<RoomSidebarPanel> = {
  serialize: (value) => value,
  parse: (raw) => {
    if (isRoomSidebarPanel(raw)) return raw;
    return undefined;
  }
};

export function roomSidebarPanelStorageSuffix(roomId: string): string {
  return `room:${roomId}:sidebarPanel`;
}

export function getRoomSidebarPanelState(serverId: string, roomId: string): RoomSidebarPanelState {
  return serverSlot(
    serverId,
    roomSidebarPanelStorageSuffix(roomId),
    ROOM_SIDEBAR_DEFAULT_PANEL,
    codec
  ).get();
}

export function setRoomSidebarPanelState(
  serverId: string,
  roomId: string,
  panel: RoomSidebarPanelState
): void {
  if (panel === null) return;

  serverSlot(
    serverId,
    roomSidebarPanelStorageSuffix(roomId),
    ROOM_SIDEBAR_DEFAULT_PANEL,
    codec
  ).set(panel);
}

export function getRoomSidebarPanel(serverId: string, roomId: string): RoomSidebarPanel {
  return getRoomSidebarPanelState(serverId, roomId) ?? ROOM_SIDEBAR_DEFAULT_PANEL;
}

export function setRoomSidebarPanel(
  serverId: string,
  roomId: string,
  panel: RoomSidebarPanel
): void {
  setRoomSidebarPanelState(serverId, roomId, panel);
}
