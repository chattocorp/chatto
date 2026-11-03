/** Administrative layout reads distinguish metadata from complete snapshots.
 * New scoped reads never fall back to an ordinary, visibility-filtered directory. */
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { updateMask } from './updateMask.js';
import { AdminRoomLayoutService } from '@chatto/api-types/admin/v1/room_layout_connect';
import {
  AdminRoomGroup,
  AdminRoomGroupLayout,
  AdminRoomLayout,
  type AdminRoomLayoutGroup
} from '@chatto/api-types/admin/v1/room_layout_pb';

type ReadOptions = { signal?: AbortSignal };

/** Create metadata, complete-layout, and link reads under their distinct gates. */
export function createAdminRoomLayoutAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminRoomLayoutService, config);
  return {
    /** Read group metadata with management or permission-editor access. */
    async getRoomGroup(groupId: string, options: ReadOptions = {}): Promise<AdminRoomGroup> {
      const response = await client.getRoomGroup({ groupId }, options);
      return requiredGroup(response.roomGroup, response.group);
    },
    /** Read the full server snapshot. Older servers use the equivalent admin read. */
    async getRoomLayout(options: ReadOptions = {}): Promise<AdminRoomLayout> {
      try {
        const response = await client.getRoomLayout({}, options);
        if (!response.layout) throw new Error('admin layout response did not include a layout');
        return response.layout;
      } catch (error) {
        if (!(error instanceof ConnectError) || error.code !== Code.Unimplemented) throw error;
        const response = await client.listRoomGroups({}, options);
        return new AdminRoomLayout({
          groups: response.groups.map(
            (group) =>
              new AdminRoomGroupLayout({
                group: requiredGroup(undefined, group),
                items: group.items
              })
          )
        });
      }
    },
    /** Read all group entries with scoped room.manage. Older servers return UNIMPLEMENTED. */
    async getRoomGroupLayout(
      groupId: string,
      options: ReadOptions = {}
    ): Promise<AdminRoomGroupLayout> {
      const response = await client.getRoomGroupLayout({ groupId }, options);
      if (!response.layout) throw new Error('admin group response did not include a layout');
      return response.layout;
    },
    /** Read a link and its owning group. Requires a server with scoped link reads. */
    async getSidebarLink(linkId: string, options: ReadOptions = {}) {
      const response = await client.getSidebarLink({ linkId }, options);
      if (!response.link) throw new Error('admin link response did not include a link');
      return response.link;
    },
    /** Create a group and return metadata without interpreting an empty layout. */
    async createRoomGroup(
      input: { name: string; description?: string },
      options: ReadOptions = {}
    ): Promise<AdminRoomGroup> {
      const response = await client.createRoomGroup(input, options);
      return requiredGroup(response.roomGroup, response.group);
    },
    /** Patch group metadata. Explicit empty descriptions clear the field. */
    async updateRoomGroup(
      input: { groupId: string; name?: string; description?: string },
      options: ReadOptions = {}
    ): Promise<AdminRoomGroup> {
      const response = await client.updateRoomGroup(
        { ...input, updateMask: updateMask(input, ['name', 'description']) },
        options
      );
      return requiredGroup(response.roomGroup, response.group);
    }
  };
}

function requiredGroup(
  group: AdminRoomGroup | undefined,
  legacy: AdminRoomLayoutGroup | undefined
): AdminRoomGroup {
  if (group) return group;
  if (!legacy) throw new Error('admin group response did not include metadata');
  return new AdminRoomGroup({ id: legacy.id, name: legacy.name, description: legacy.description });
}
