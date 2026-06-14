import { serverSlot, type Codec } from './slot';

export const ROOM_SIDEBAR_PANELS = ['members', 'files'] as const;

export type RoomSidebarPanel = (typeof ROOM_SIDEBAR_PANELS)[number];

export const ROOM_SIDEBAR_DEFAULT_PANEL: RoomSidebarPanel = 'members';

function isRoomSidebarPanel(value: unknown): value is RoomSidebarPanel {
  return typeof value === 'string' && ROOM_SIDEBAR_PANELS.includes(value as RoomSidebarPanel);
}

const codec: Codec<RoomSidebarPanel> = {
  serialize: (value) => value,
  parse: (raw) => (isRoomSidebarPanel(raw) ? raw : undefined)
};

export function roomSidebarPanelStorageSuffix(roomId: string): string {
  return `room:${roomId}:sidebarPanel`;
}

export function getRoomSidebarPanel(serverId: string, roomId: string): RoomSidebarPanel {
  return serverSlot(
    serverId,
    roomSidebarPanelStorageSuffix(roomId),
    ROOM_SIDEBAR_DEFAULT_PANEL,
    codec
  ).get();
}

export function setRoomSidebarPanel(
  serverId: string,
  roomId: string,
  panel: RoomSidebarPanel
): void {
  serverSlot(
    serverId,
    roomSidebarPanelStorageSuffix(roomId),
    ROOM_SIDEBAR_DEFAULT_PANEL,
    codec
  ).set(panel);
}
