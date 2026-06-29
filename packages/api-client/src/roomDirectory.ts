import { notifyAuthenticationRequired } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { RoomDirectoryService } from "@chatto/api-types/api/v1/room_directory_connect";
import type {
  DirectoryRoom,
  RoomGroup,
  RoomGroupItem,
} from "@chatto/api-types/api/v1/room_directory_pb";
import { RoomDirectoryScope } from "@chatto/api-types/api/v1/room_directory_pb";
import { RoomKind } from "@chatto/api-types/api/v1/rooms_pb";

export type RoomDirectoryAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

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
      type: "room";
      roomId: string;
    }
  | {
      id: string;
      type: "link";
      link: DirectorySidebarLink;
    };

export type DirectoryRoomGroup = {
  id: string;
  name: string;
  roomIds: string[];
  items: DirectoryRoomGroupItem[];
};

export { RoomDirectoryScope };
export { RoomKind };

export function createRoomDirectoryAPI(config: RoomDirectoryAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const directory = createClient(RoomDirectoryService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      config.serverId
    ) {
      notifyAuthenticationRequired(
        config.serverId,
        config.onAuthenticationRequired,
      );
    }
    throw err;
  }

  return {
    async listRooms(
      scope: RoomDirectoryScope,
    ): Promise<DirectoryRoomSummary[]> {
      try {
        const response = await directory.listRooms(
          { scope },
          { headers: headers() },
        );
        return response.rooms.flatMap((entry) => mapDirectoryRoom(entry) ?? []);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async getRoom(roomId: string): Promise<DirectoryRoomDetails | null> {
      try {
        const response = await directory.getRoom(
          { roomId },
          { headers: headers() },
        );
        return mapDirectoryRoomDetails(response.room);
      } catch (err) {
        if (
          err instanceof ConnectError &&
          (err.code === Code.NotFound || err.code === Code.PermissionDenied)
        ) {
          return null;
        }
        return handleAuthError(err);
      }
    },

    async listRoomGroups(): Promise<DirectoryRoomGroup[]> {
      try {
        const response = await directory.listRoomGroups(
          {},
          { headers: headers() },
        );
        return response.groups.map(mapRoomGroup);
      } catch (err) {
        return handleAuthError(err);
      }
    },
  };
}

export type RoomDirectoryAPI = ReturnType<typeof createRoomDirectoryAPI>;

function mapDirectoryRoomDetails(
  entry: DirectoryRoom | undefined,
): DirectoryRoomDetails | null {
  const summary = entry ? mapDirectoryRoom(entry) : null;
  if (!summary) return null;

  return {
    ...summary,
    canPostMessage: entry?.viewerState?.canPostMessage ?? false,
    canPostInThread: entry?.viewerState?.canPostInThread ?? false,
    canAttach: entry?.viewerState?.canAttach ?? false,
    canReact: entry?.viewerState?.canReact ?? false,
    canEchoMessage: entry?.viewerState?.canEchoMessage ?? false,
    canManageOthersMessage: entry?.viewerState?.canManageOthersMessage ?? false,
    canManageRoom: entry?.viewerState?.canManageRoom ?? false,
    canBanRoomMembers: entry?.viewerState?.canBanRoomMembers ?? false,
  };
}

function mapDirectoryRoom(entry: DirectoryRoom): DirectoryRoomSummary | null {
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
    canJoinRoom: entry.viewerState?.canJoinRoom ?? false,
  };
}

function mapRoomGroup(group: RoomGroup): DirectoryRoomGroup {
  return {
    id: group.id,
    name: group.name,
    roomIds: uniqueRoomIds(group.rooms),
    items: sidebarItemsFromAPI(group),
  };
}

function uniqueRoomIds(rooms: readonly DirectoryRoom[]): string[] {
  const seen: Record<string, true> = Object.create(null);
  return rooms.flatMap((entry) => {
    const id = entry.room?.id;
    if (!id || seen[id]) return [];
    seen[id] = true;
    return [id];
  });
}

function sidebarItemsFromAPI(group: RoomGroup): DirectoryRoomGroupItem[] {
  if (group.items.length === 0) {
    return uniqueRoomIds(group.rooms).map((roomId) => ({
      id: `room:${roomId}`,
      type: "room",
      roomId,
    }));
  }

  return group.items.flatMap((item) => mapRoomGroupItem(item) ?? []);
}

function mapRoomGroupItem(item: RoomGroupItem): DirectoryRoomGroupItem | null {
  if (item.item.case === "room") {
    const roomId = item.item.value.room?.id;
    return roomId ? { id: `room:${roomId}`, type: "room", roomId } : null;
  }
  if (item.item.case === "sidebarLink") {
    return {
      id: `link:${item.item.value.id}`,
      type: "link",
      link: {
        id: item.item.value.id,
        label: item.item.value.label,
        url: item.item.value.url,
      },
    };
  }
  return null;
}
