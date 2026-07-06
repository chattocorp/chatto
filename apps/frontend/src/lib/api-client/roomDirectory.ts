import {
  authHeaders,
  Code,
  ConnectError,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig
} from './connect.js';
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

export type RoomDirectoryAPIConfig = ConnectAPIConfig;

export type DirectoryRoomSummary = {
  id: string;
  name: string;
  description: string | null;
  kind: RoomKind;
  archived: boolean;
  isUniversal: boolean;
  isMember: boolean;
  hasUnread: boolean;
  canJoinRoom: boolean;
};

export type DirectoryRoomDetails = DirectoryRoomSummary & {
  canPostMessage: boolean;
  canPostInThread: boolean;
  canAttach: boolean;
  canReact: boolean;
  canEchoMessage: boolean;
  canManageOthersMessage: boolean;
  canManageRoom: boolean;
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
  roomIds: string[];
  items: DirectoryRoomGroupItem[];
};

export { RoomDirectoryScope };
export { RoomKind };

const RoomPermission = {
  Attach: 'message.attach',
  BanMember: 'room.ban-member',
  CreateRoom: 'room.create',
  EchoMessage: 'message.echo',
  JoinRoom: 'room.join',
  ManageMessage: 'message.manage',
  ManageRoom: 'room.manage',
  PostInThread: 'message.post-in-thread',
  PostMessage: 'message.post',
  React: 'message.react'
} as const;

export function createRoomDirectoryAPI(config: RoomDirectoryAPIConfig) {
  const directory = createChattoClient(RoomDirectoryService, config);
  const headers = () => authHeaders(config);

  return {
    async listRooms(scope: RoomDirectoryScope): Promise<DirectoryRoomSummary[]> {
      try {
        const response = await directory.listRooms({ scope }, { headers: headers() });
        return response.rooms.flatMap((entry) => mapDirectoryRoom(entry) ?? []);
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async getRoom(roomId: string): Promise<DirectoryRoomDetails | null> {
      try {
        const response = await directory.getRoom({ roomId }, { headers: headers() });
        return mapDirectoryRoomDetails(response.room);
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        return handleAuthError(config, err);
      }
    },

    async batchGetRooms(roomIds: string[]): Promise<DirectoryRoomDetails[]> {
      try {
        const response = await directory.batchGetRooms({ roomIds }, { headers: headers() });
        return response.rooms.flatMap((entry) => {
          const mapped = mapDirectoryRoomDetails(entry);
          return mapped ? [mapped] : [];
        });
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async listRoomGroups(): Promise<DirectoryRoomGroup[]> {
      try {
        const response = await directory.listRoomGroups({}, { headers: headers() });
        return response.groups.map(mapRoomGroup);
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async getRoomGroup(groupId: string): Promise<DirectoryRoomGroup | null> {
      try {
        const response = await directory.getRoomGroup({ groupId }, { headers: headers() });
        return response.group ? mapRoomGroup(response.group) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        return handleAuthError(config, err);
      }
    },

    async batchGetRoomGroups(groupIds: string[]): Promise<DirectoryRoomGroup[]> {
      try {
        const response = await directory.batchGetRoomGroups({ groupIds }, { headers: headers() });
        return response.groups.map(mapRoomGroup);
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

export type RoomDirectoryAPI = ReturnType<typeof createRoomDirectoryAPI>;

function mapDirectoryRoomDetails(
  entry: RoomWithViewerState | undefined
): DirectoryRoomDetails | null {
  if (!entry) return null;

  const summary = mapDirectoryRoom(entry);
  if (!summary) return null;

  return {
    ...summary,
    canPostMessage: hasRoomPermission(entry.viewerState, RoomPermission.PostMessage),
    canPostInThread: hasRoomPermission(entry.viewerState, RoomPermission.PostInThread),
    canAttach: hasRoomPermission(entry.viewerState, RoomPermission.Attach),
    canReact: hasRoomPermission(entry.viewerState, RoomPermission.React),
    canEchoMessage: hasRoomPermission(entry.viewerState, RoomPermission.EchoMessage),
    canManageOthersMessage: hasRoomPermission(entry.viewerState, RoomPermission.ManageMessage),
    canManageRoom: hasRoomPermission(entry.viewerState, RoomPermission.ManageRoom),
    canBanRoomMembers: hasRoomPermission(entry.viewerState, RoomPermission.BanMember)
  };
}

function mapDirectoryRoom(entry: RoomWithViewerState): DirectoryRoomSummary | null {
  if (!entry.room) return null;
  return {
    id: entry.room.id,
    name: entry.room.name,
    description: entry.room.description || null,
    kind: entry.room.kind,
    archived: entry.room.archived,
    isUniversal: entry.room.universal,
    isMember: entry.viewerState?.isMember ?? false,
    hasUnread: entry.viewerState?.hasUnread ?? false,
    canJoinRoom: hasRoomPermission(entry.viewerState, RoomPermission.JoinRoom)
  };
}

function mapRoomGroup(group: RoomGroup): DirectoryRoomGroup {
  return {
    id: group.id,
    name: group.name,
    canCreateRoom: hasRoomGroupPermission(group.viewerState, RoomPermission.CreateRoom),
    roomIds: uniqueRoomIds(group.items),
    items: sidebarItemsFromAPI(group)
  };
}

function hasRoomPermission(state: RoomViewerState | undefined, permission: string): boolean {
  return (
    state?.permissions.some((grant) => grant.permission === permission && grant.granted) ?? false
  );
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
