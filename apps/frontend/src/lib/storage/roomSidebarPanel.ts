import { serverSlot, type Codec } from './slot';
import { serverStorageKey } from './serverStorage';

export const ROOM_SIDEBAR_PANELS = ['members', 'files', 'call'] as const;

export type RoomSidebarPanel = (typeof ROOM_SIDEBAR_PANELS)[number];
export type RoomSidebarPanelState = RoomSidebarPanel | null;

export const ROOM_SIDEBAR_DEFAULT_PANEL: RoomSidebarPanel = 'members';
const PENDING_ROOM_SIDEBAR_PANEL_SUFFIX = 'roomSidebarPanel:pendingOpen';

type PendingRoomSidebarPanel = {
  roomId: string;
  panel: RoomSidebarPanel;
};

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

export function setPendingRoomSidebarPanel(
  serverId: string,
  roomId: string,
  panel: RoomSidebarPanel
): void {
  const storage = getSessionStorage();
  if (!storage) return;

  storage.setItem(
    serverSlotKey(serverId),
    JSON.stringify({
      roomId,
      panel
    } satisfies PendingRoomSidebarPanel)
  );
}

export function consumePendingRoomSidebarPanel(
  serverId: string,
  roomId: string
): RoomSidebarPanelState {
  const storage = getSessionStorage();
  if (!storage) return null;

  const key = serverSlotKey(serverId);
  const raw = storage.getItem(key);
  if (!raw) return null;

  let pending: Partial<PendingRoomSidebarPanel>;
  try {
    pending = JSON.parse(raw) as Partial<PendingRoomSidebarPanel>;
  } catch {
    storage.removeItem(key);
    return null;
  }

  if (pending.roomId !== roomId) return null;
  storage.removeItem(key);
  return isRoomSidebarPanel(pending.panel) ? pending.panel : null;
}

function serverSlotKey(serverId: string): string {
  return serverStorageKey(serverId, PENDING_ROOM_SIDEBAR_PANEL_SUFFIX);
}

function getSessionStorage(): Storage | null {
  try {
    return globalThis.sessionStorage ?? null;
  } catch {
    return null;
  }
}
