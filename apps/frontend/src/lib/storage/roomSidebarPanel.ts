import { serverSlot, type Codec } from './slot';

export const ROOM_SIDEBAR_PANELS = ['members', 'search', 'files', 'pins', 'call'] as const;

export type RoomSidebarPanel = (typeof ROOM_SIDEBAR_PANELS)[number];
export type RoomSidebarPanelState = RoomSidebarPanel | null;

/** A desktop profile retains the extras panel to show when the profile closes. */
export type RoomSidebarProfilePreference = {
  view: 'profile';
  previousPanel: RoomSidebarPanelState;
};

/** Undefined means no saved choice; null means the user closed the sidebar. */
export type RoomSidebarPreference =
  RoomSidebarPanelState | RoomSidebarProfilePreference | undefined;

function isRoomSidebarPanel(value: unknown): value is RoomSidebarPanel {
  return typeof value === 'string' && ROOM_SIDEBAR_PANELS.includes(value as RoomSidebarPanel);
}

// Keep this startup codec small: the root app UI also loads on the login page.
const codec: Codec<Exclude<RoomSidebarPreference, undefined>> = {
  serialize: (value) => {
    if (value === null) return 'closed';
    if (typeof value === 'string') return value;
    return `profile:${value.previousPanel ?? 'closed'}`;
  },
  parse: (raw) => {
    if (isRoomSidebarPanel(raw)) return raw;
    if (raw === 'closed') return null;
    if (raw.startsWith('profile:')) {
      const previousPanel = raw.slice('profile:'.length);
      if (previousPanel === 'closed' || isRoomSidebarPanel(previousPanel)) {
        return {
          view: 'profile',
          previousPanel: previousPanel === 'closed' ? null : previousPanel
        };
      }
    }
    return undefined;
  }
};

export function roomSidebarPanelStorageSuffix(roomId: string): string {
  return `room:${roomId}:sidebarPanel`;
}

/** Read a desktop preference without applying a room-specific default. */
export function getRoomSidebarPanelState(serverId: string, roomId: string): RoomSidebarPreference {
  return serverSlot(serverId, roomSidebarPanelStorageSuffix(roomId), undefined, codec).get();
}

/** Save an explicit desktop choice, including closed state. */
export function setRoomSidebarPanelState(
  serverId: string,
  roomId: string,
  panel: Exclude<RoomSidebarPreference, undefined>
): void {
  serverSlot(serverId, roomSidebarPanelStorageSuffix(roomId), null, codec).set(panel);
}
