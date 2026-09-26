import { listAllDirectoryRooms } from './roomPages';
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import type {
  RoomGroup,
  RoomGroupItem,
  RoomGroupViewerState,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomDirectoryScope } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { normalizeRoomThreadingMode, type RoomThreadingMode } from '$lib/roomThreading';


export type DirectoryRoomSummary = {
  id: string;
  name: string;
  description: string | null;
  kind: RoomKind;
  archived: boolean;
  isUniversal: boolean;
  slowModeSeconds: number;
  threadingMode: RoomThreadingMode;
  slowModeNextPostAt: string | null;
  isMember: boolean;
  hasUnread: boolean;
  canReadMessages: boolean | null;
  canJoinRoom: boolean;
  canManageRoom: boolean;
};

export type DirectoryRoomDetails = DirectoryRoomSummary & {
  /** True only when the server explicitly grants interaction-only message access. */
  hasLimitedMessageAccess: boolean;
  canPostMessage: boolean;
  canPostInThread: boolean;
  /** Permission gate only; the server checks the relationship for each thread. */
  canPostInteractions: boolean;
  canAttach: boolean;
  canReact: boolean;
  canEchoMessage: boolean;
  canManageOthersMessage: boolean;
  canBanRoomMembers: boolean;
};

export type DirectorySidebarLink = {
  id: string;
  label: string;
  url: string;
};

export type DirectoryRoomGroupItem =
  | {
      id: string;
      type: 'room';
      roomId: string;
      room: DirectoryRoomSummary;
    }
  | {
      id: string;
      type: 'link';
      link: DirectorySidebarLink;
    };

export type DirectoryRoomGroup = {
  id: string;
  name: string;
  canCreateRoom: boolean;
  canManageGroup: boolean;
  roomIds: string[];
  items: DirectoryRoomGroupItem[];
};

export { RoomDirectoryScope };
export { RoomKind };

const RoomPermission = {
  Attach: 'message.attach',
  BanMember: 'room.remove-member',
  CreateRoom: 'room.create',
  EchoMessage: 'message.echo',
  JoinRoom: 'room.join',
  ManageMessage: 'message.manage',
  ManageRoom: 'room.manage',
  ReadInteractions: 'message.read-interactions',
  ReadMessages: 'message.read',
  PostInThread: 'message.post-in-thread',
  PostInteractions: 'message.post-in-interactions',
  PostMessage: 'message.post',
  React: 'message.react'
} as const;

export function createRoomDirectoryAPI(config: ConnectAPIConfig) {
  const directory = createChattoClient(RoomDirectoryService, config);

  return {
    async listRooms(
      scope: RoomDirectoryScope,
      options: { signal?: AbortSignal } = {}
    ): Promise<DirectoryRoomSummary[]> {
      const rooms = await listAllDirectoryRooms((page) =>
        directory.listRooms(
          { scope, page },
          { ...(options.signal ? { signal: options.signal } : {}) }
        )
      );
      return rooms.flatMap((entry) => mapDirectoryRoom(entry) ?? []);
    },

    async getRoom(roomId: string): Promise<DirectoryRoomDetails | null> {
      try {
        const response = await directory.getRoom({ roomId });
        return mapDirectoryRoomDetails(response.room);
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetRooms(
      roomIds: string[],
      options: { signal?: AbortSignal } = {}
    ): Promise<DirectoryRoomDetails[]> {
      const response = await directory.batchGetRooms({ roomIds }, { signal: options.signal });
      return response.rooms.flatMap((entry) => {
        const mapped = mapDirectoryRoomDetails(entry);
        return mapped ? [mapped] : [];
      });
    },

    async listRoomGroups(): Promise<DirectoryRoomGroup[]> {
      const response = await directory.listRoomGroups({});
      return response.groups.map(mapRoomGroup);
    },

    async getRoomGroup(groupId: string): Promise<DirectoryRoomGroup | null> {
      try {
        const response = await directory.getRoomGroup({ groupId });
        return response.group ? mapRoomGroup(response.group) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetRoomGroups(groupIds: string[]): Promise<DirectoryRoomGroup[]> {
      const response = await directory.batchGetRoomGroups({ groupIds });
      return response.groups.map(mapRoomGroup);
    }
  };
}

export type RoomDirectoryAPI = ReturnType<typeof createRoomDirectoryAPI>;

export function mapDirectoryRoomDetails(
  entry: RoomWithViewerState | undefined
): DirectoryRoomDetails | null {
  if (!entry) return null;

  const summary = mapDirectoryRoom(entry);
  if (!summary) return null;

  return {
    ...summary,
    hasLimitedMessageAccess:
      roomPermissionDecision(entry.viewerState, RoomPermission.ReadMessages) === false &&
      hasRoomPermission(entry.viewerState, RoomPermission.ReadInteractions),
    canPostMessage: hasRoomPermission(entry.viewerState, RoomPermission.PostMessage),
    canPostInThread: hasRoomPermission(entry.viewerState, RoomPermission.PostInThread),
    canPostInteractions: hasRoomPermission(entry.viewerState, RoomPermission.PostInteractions),
    canAttach: hasRoomPermission(entry.viewerState, RoomPermission.Attach),
    canReact: hasRoomPermission(entry.viewerState, RoomPermission.React),
    canEchoMessage: hasRoomPermission(entry.viewerState, RoomPermission.EchoMessage),
    canManageOthersMessage: hasRoomPermission(entry.viewerState, RoomPermission.ManageMessage),
    canManageRoom: hasRoomPermission(entry.viewerState, RoomPermission.ManageRoom),
    canBanRoomMembers: hasRoomPermission(entry.viewerState, RoomPermission.BanMember)
  };
}

export function mapDirectoryRoom(entry: RoomWithViewerState): DirectoryRoomSummary | null {
  if (!entry.room) return null;
  return {
    id: entry.room.id,
    name: entry.room.name,
    description: entry.room.description || null,
    kind: entry.room.kind,
    archived: entry.room.archived,
    isUniversal: entry.room.universal,
    slowModeSeconds: entry.room.slowModeSeconds ?? 0,
    threadingMode: normalizeRoomThreadingMode(entry.room.kind, entry.room.threadingMode),
    slowModeNextPostAt: entry.viewerState?.slowModeNextPostAt?.toDate().toISOString() ?? null,
    isMember: entry.viewerState?.isMember ?? false,
    hasUnread: entry.viewerState?.hasUnread ?? false,
    canReadMessages: anyRoomPermissionDecision(entry.viewerState, [
      RoomPermission.ReadMessages,
      RoomPermission.ReadInteractions
    ]),
    canJoinRoom: hasRoomPermission(entry.viewerState, RoomPermission.JoinRoom),
    canManageRoom: hasRoomPermission(entry.viewerState, RoomPermission.ManageRoom)
  };
}

export function mapRoomGroup(group: RoomGroup): DirectoryRoomGroup {
  return {
    id: group.id,
    name: group.name,
    canCreateRoom: hasRoomGroupPermission(group.viewerState, RoomPermission.CreateRoom),
    canManageGroup: hasRoomGroupPermission(group.viewerState, RoomPermission.ManageRoom),
    roomIds: uniqueRoomIds(group.items),
    items: sidebarItemsFromAPI(group)
  };
}

function hasRoomPermission(state: RoomViewerState | undefined, permission: string): boolean {
  return roomPermissionDecision(state, permission) ?? false;
}

function roomPermissionDecision(
  state: RoomViewerState | undefined,
  permission: string
): boolean | null {
  const grant = state?.permissions.find((candidate) => candidate.permission === permission);
  return grant ? grant.granted : null;
}

function anyRoomPermissionDecision(
  state: RoomViewerState | undefined,
  permissions: readonly string[]
): boolean | null {
  const decisions = permissions.map((permission) => roomPermissionDecision(state, permission));
  if (decisions.some((decision) => decision === true)) return true;
  if (decisions.some((decision) => decision === false)) return false;
  return null;
}

function hasRoomGroupPermission(
  state: RoomGroupViewerState | undefined,
  permission: string
): boolean {
  return (
    state?.permissions.some((grant) => grant.permission === permission && grant.granted) ?? false
  );
}

function uniqueRoomIds(items: readonly RoomGroupItem[]): string[] {
  const seen: Record<string, true> = Object.create(null);
  return items.flatMap((item) => {
    if (item.item.case !== 'room') return [];
    const id = item.item.value.room?.id;
    if (!id || seen[id]) return [];
    seen[id] = true;
    return [id];
  });
}

function sidebarItemsFromAPI(group: RoomGroup): DirectoryRoomGroupItem[] {
  return group.items.flatMap((item) => mapRoomGroupItem(item) ?? []);
}

function mapRoomGroupItem(item: RoomGroupItem): DirectoryRoomGroupItem | null {
  if (item.item.case === 'room') {
    const roomId = item.item.value.room?.id;
    const room = mapDirectoryRoom(item.item.value);
    return roomId && room ? { id: `room:${roomId}`, type: 'room', roomId, room } : null;
  }
  if (item.item.case === 'sidebarLink') {
    return {
      id: `link:${item.item.value.id}`,
      type: 'link',
      link: {
        id: item.item.value.id,
        label: item.item.value.label,
        url: item.item.value.url
      }
    };
  }
  return null;
}
