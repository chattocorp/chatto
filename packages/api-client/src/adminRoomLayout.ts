import { notifyAuthenticationRequired } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AdminRoomLayoutService } from "@chatto/api-types/admin/v1/room_layout_connect";
import {
  AdminRoomLayoutItemKind,
  type AdminRoomLayoutGroup as APIAdminRoomLayoutGroup,
  type AdminRoomLayoutItem as APIAdminRoomLayoutItem,
} from "@chatto/api-types/admin/v1/room_layout_pb";
import type { SidebarLink } from "@chatto/api-types/api/v1/room_directory_pb";
import type { Room } from "@chatto/api-types/api/v1/rooms_pb";

export type AdminRoomLayoutAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type AdminRoomInfo = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  isUniversal: boolean;
};

export type AdminSidebarLinkInfo = {
  id: string;
  label: string;
  url: string;
};

export type AdminSidebarItem =
  | {
      id: string;
      kind: "room";
      room: AdminRoomInfo;
    }
  | {
      id: string;
      kind: "link";
      link: AdminSidebarLinkInfo;
    };

export type AdminRoomGroup = {
  id: string;
  name: string;
  rooms: AdminRoomInfo[];
  items?: AdminSidebarItem[];
};

export type AdminRoomLayoutItemMutationInput = {
  kind: AdminSidebarItem["kind"];
  id: string;
};

export function createAdminRoomLayoutAPI(config: AdminRoomLayoutAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const layout = createClient(AdminRoomLayoutService, transport);
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
    async listAdminRoomLayout(): Promise<AdminRoomGroup[]> {
      try {
        const response = await layout.listAdminRoomLayout(
          {},
          { headers: headers() },
        );
        return response.groups.map(mapAdminRoomLayoutGroup);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async createRoomGroup(input: {
      name: string;
      description?: string | null;
    }): Promise<AdminRoomGroup | null> {
      try {
        const response = await layout.createRoomGroup(
          { name: input.name, description: input.description ?? "" },
          { headers: headers() },
        );
        return response.group ? mapAdminRoomLayoutGroup(response.group) : null;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async updateRoomGroup(input: {
      groupId: string;
      name: string;
      description?: string | null;
    }): Promise<AdminRoomGroup | null> {
      try {
        const response = await layout.updateRoomGroup(
          {
            groupId: input.groupId,
            name: input.name,
            description: input.description ?? "",
          },
          { headers: headers() },
        );
        return response.group ? mapAdminRoomLayoutGroup(response.group) : null;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async deleteRoomGroup(groupId: string): Promise<boolean> {
      try {
        const response = await layout.deleteRoomGroup(
          { groupId },
          { headers: headers() },
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async reorderRoomGroups(
      orderedGroupIds: string[],
    ): Promise<AdminRoomGroup[]> {
      try {
        const response = await layout.reorderRoomGroups(
          { orderedGroupIds },
          { headers: headers() },
        );
        return response.groups.map(mapAdminRoomLayoutGroup);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async moveRoomToGroup(input: {
      roomId: string;
      groupId: string;
    }): Promise<void> {
      try {
        await layout.moveRoomToGroup(input, { headers: headers() });
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async reorderSidebarItemsInGroup(input: {
      groupId: string;
      items: AdminRoomLayoutItemMutationInput[];
    }): Promise<AdminRoomGroup | null> {
      try {
        const response = await layout.reorderSidebarItemsInGroup(
          {
            groupId: input.groupId,
            items: input.items.map((item) => ({
              id: item.id,
              kind:
                item.kind === "room"
                  ? AdminRoomLayoutItemKind.ROOM
                  : AdminRoomLayoutItemKind.SIDEBAR_LINK,
            })),
          },
          { headers: headers() },
        );
        return response.group ? mapAdminRoomLayoutGroup(response.group) : null;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async createSidebarLink(input: {
      groupId: string;
      label: string;
      url: string;
    }): Promise<AdminSidebarLinkInfo | null> {
      try {
        const response = await layout.createSidebarLink(input, {
          headers: headers(),
        });
        return response.sidebarLink
          ? mapSidebarLink(response.sidebarLink)
          : null;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async updateSidebarLink(input: {
      linkId: string;
      label: string;
      url: string;
    }): Promise<AdminSidebarLinkInfo | null> {
      try {
        const response = await layout.updateSidebarLink(input, {
          headers: headers(),
        });
        return response.sidebarLink
          ? mapSidebarLink(response.sidebarLink)
          : null;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async deleteSidebarLink(linkId: string): Promise<boolean> {
      try {
        const response = await layout.deleteSidebarLink(
          { linkId },
          { headers: headers() },
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async moveSidebarLinkToGroup(input: {
      linkId: string;
      groupId: string;
    }): Promise<void> {
      try {
        await layout.moveSidebarLinkToGroup(input, { headers: headers() });
      } catch (err) {
        return handleAuthError(err);
      }
    },
  };
}

export type AdminRoomLayoutAPI = ReturnType<typeof createAdminRoomLayoutAPI>;

function mapAdminRoomLayoutGroup(
  group: APIAdminRoomLayoutGroup,
): AdminRoomGroup {
  const rooms = (group.rooms ?? []).map(mapAdminRoom);
  const items =
    (group.items ?? []).length > 0
      ? (group.items ?? []).flatMap(
          (item) => mapAdminRoomLayoutItem(item) ?? [],
        )
      : rooms.map((room) => ({
          id: `room:${room.id}`,
          kind: "room" as const,
          room,
        }));
  return {
    id: group.id,
    name: group.name,
    rooms,
    items,
  };
}

function mapAdminRoomLayoutItem(
  item: APIAdminRoomLayoutItem,
): AdminSidebarItem | null {
  if (item.item.case === "room") {
    const room = mapAdminRoom(item.item.value);
    return { id: `room:${room.id}`, kind: "room", room };
  }
  if (item.item.case === "sidebarLink") {
    const link = mapSidebarLink(item.item.value);
    return { id: `link:${link.id}`, kind: "link", link };
  }
  return null;
}

function mapAdminRoom(room: Room): AdminRoomInfo {
  return {
    id: room.id,
    name: room.name,
    description: room.description || null,
    archived: room.archived ?? false,
    isUniversal: room.universal ?? false,
  };
}

function mapSidebarLink(link: SidebarLink): AdminSidebarLinkInfo {
  return {
    id: link.id,
    label: link.label,
    url: link.url,
  };
}
