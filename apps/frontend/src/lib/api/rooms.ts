import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { Timestamp } from '@bufbuild/protobuf';
import { RoomService } from '$lib/pb/chatto/api/v1/rooms_connect';
import type { Room, RoomBan as APIRoomBan } from '$lib/pb/chatto/api/v1/rooms_pb';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { mapDirectoryMember, type DirectoryMember } from './memberDirectory';

export type ConnectAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
};

export type PublicRoom = {
  id: string;
  name: string;
  description: string;
  archived: boolean;
  groupId: string;
  universal: boolean;
};

export type RoomBanSummary = {
  id: string;
  roomId: string;
  room: PublicRoom | null;
  userId: string;
  user: DirectoryMember | null;
  moderatorId: string;
  moderator: DirectoryMember | null;
  reason: string;
  createdAt: string | null;
  expiresAt: string | null;
};

export type RoomBanList = {
  bans: RoomBanSummary[];
  totalCount: number;
  hasMore: boolean;
};

export type RoomCommandAPI = ReturnType<typeof createRoomCommandAPI>;

const ROOM_NAME_MAX_LENGTH = 30;
const ROOM_DESCRIPTION_MAX_LENGTH = 500;

function publicRoom(room: Room | undefined): PublicRoom | null {
  if (!room) return null;
  return {
    id: room.id,
    name: room.name,
    description: room.description,
    archived: room.archived,
    groupId: room.groupId,
    universal: room.universal
  };
}

function roomBan(ban: APIRoomBan): RoomBanSummary {
  return {
    id: ban.id,
    roomId: ban.roomId,
    room: publicRoom(ban.room),
    userId: ban.userId,
    user: ban.user ? mapDirectoryMember(ban.user) : null,
    moderatorId: ban.moderatorId,
    moderator: ban.moderator ? mapDirectoryMember(ban.moderator) : null,
    reason: ban.reason,
    createdAt: ban.createdAt?.toDate().toISOString() ?? null,
    expiresAt: ban.expiresAt?.toDate().toISOString() ?? null
  };
}

function roomValidationError(err: unknown, input: { name: string; description?: string | null }) {
  if (!(err instanceof ConnectError) || err.code !== Code.InvalidArgument) return err;

  if (input.name.length > ROOM_NAME_MAX_LENGTH) {
    return new Error(`room name must be ${ROOM_NAME_MAX_LENGTH} characters or less`);
  }
  if ((input.description ?? '').length > ROOM_DESCRIPTION_MAX_LENGTH) {
    return new Error(`room description must be ${ROOM_DESCRIPTION_MAX_LENGTH} characters or less`);
  }

  return err;
}

export function createRoomCommandAPI(config: ConnectAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const rooms = createClient(RoomService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
      serverRegistry.handleAuthenticationRequired(config.serverId);
    }
    throw err;
  }

  return {
    async createRoom(input: {
      name: string;
      description?: string | null;
      groupId: string;
      universal?: boolean;
    }): Promise<PublicRoom | null> {
      try {
        const response = await rooms.createRoom(
          {
            name: input.name,
            description: input.description ?? '',
            groupId: input.groupId,
            universal: input.universal ?? false
          },
          { headers: headers() }
        );
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(roomValidationError(err, input));
      }
    },

    async updateRoom(input: {
      roomId: string;
      name: string;
      description?: string | null;
    }): Promise<PublicRoom | null> {
      try {
        const response = await rooms.updateRoom(
          {
            roomId: input.roomId,
            name: input.name,
            description: input.description ?? ''
          },
          { headers: headers() }
        );
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(roomValidationError(err, input));
      }
    },

    async archiveRoom(roomId: string): Promise<PublicRoom | null> {
      try {
        const response = await rooms.archiveRoom({ roomId }, { headers: headers() });
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async unarchiveRoom(roomId: string): Promise<PublicRoom | null> {
      try {
        const response = await rooms.unarchiveRoom({ roomId }, { headers: headers() });
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async setRoomUniversal(roomId: string, universal: boolean): Promise<PublicRoom | null> {
      try {
        const response = await rooms.setRoomUniversal(
          { roomId, universal },
          { headers: headers() }
        );
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async joinRoom(roomId: string): Promise<PublicRoom | null> {
      try {
        const response = await rooms.joinRoom({ roomId }, { headers: headers() });
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async startDM(participantIds: string[]): Promise<PublicRoom | null> {
      try {
        const response = await rooms.startDM({ participantIds }, { headers: headers() });
        return publicRoom(response.room);
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async leaveRoom(roomId: string): Promise<boolean> {
      try {
        const response = await rooms.leaveRoom({ roomId }, { headers: headers() });
        return response.left;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async listRoomBans(input: { roomId?: string; limit?: number; offset?: number } = {}): Promise<RoomBanList> {
      try {
        const response = await rooms.listRoomBans(
          {
            roomId: input.roomId ?? '',
            page: { limit: input.limit ?? 100, offset: input.offset ?? 0 }
          },
          { headers: headers() }
        );
        return {
          bans: response.bans.map(roomBan),
          totalCount: Number(response.page?.totalCount ?? 0),
          hasMore: response.page?.hasMore ?? false
        };
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async joinGroup(groupId: string): Promise<string[]> {
      try {
        const response = await rooms.joinRoomGroup({ groupId }, { headers: headers() });
        return response.joinedRoomIds;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async banRoomMember(input: {
      roomId: string;
      userId: string;
      reason: string;
      expiresAt?: string | null;
    }): Promise<boolean> {
      try {
        const response = await rooms.banRoomMember(
          {
            roomId: input.roomId,
            userId: input.userId,
            reason: input.reason,
            expiresAt: input.expiresAt ? Timestamp.fromDate(new Date(input.expiresAt)) : undefined
          },
          { headers: headers() }
        );
        return response.banned;
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async unbanRoomMember(input: {
      roomId: string;
      userId: string;
      reason: string;
    }): Promise<boolean> {
      try {
        const response = await rooms.unbanRoomMember(input, { headers: headers() });
        return response.unbanned;
      } catch (err) {
        return handleAuthError(err);
      }
    }
  };
}
