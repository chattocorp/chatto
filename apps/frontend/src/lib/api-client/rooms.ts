import { updateMask } from './updateMask';
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { Timestamp } from '@bufbuild/protobuf';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import type { Room, RoomSuspension as APIRoomSuspension } from '@chatto/api-types/api/v1/rooms_pb';
import { mapDirectoryMember, type DirectoryMember } from './memberDirectory.js';
import {
  normalizeRoomName,
  ROOM_NAME_MAX_LENGTH,
  roomNameCharacterCount
} from '$lib/utils/roomName';
import { normalizeRoomThreadingMode, type RoomThreadingMode } from '$lib/roomThreading';

export type { ConnectAPIConfig } from './connect.js';

export type PublicRoom = {
  id: string;
  name: string;
  description: string;
  archived: boolean;
  groupId: string;
  universal: boolean;
  slowModeSeconds: number;
  threadingMode: RoomThreadingMode;
};

export type RoomSuspensionSummary = {
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

export type RoomSuspensionList = {
  suspensions: RoomSuspensionSummary[];
  totalCount: number;
  hasMore: boolean;
};

export type RoomSuspensionChoice =
  { kind: 'none' } | { kind: 'indefinite' } | { kind: 'until'; expiresAt: string };

export type RoomCommandAPI = ReturnType<typeof createRoomCommandAPI>;

const ROOM_DESCRIPTION_MAX_LENGTH = 500;

function publicRoom(room: Room | undefined): PublicRoom | null {
  if (!room) return null;
  return {
    id: room.id,
    name: room.name,
    description: room.description,
    archived: room.archived,
    groupId: room.groupId,
    universal: room.universal,
    slowModeSeconds: room.slowModeSeconds ?? 0,
    threadingMode: normalizeRoomThreadingMode(room.kind, room.threadingMode)
  };
}

function roomSuspension(ban: APIRoomSuspension): RoomSuspensionSummary {
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

function roomValidationError(err: unknown, input: { name?: string; description?: string | null }) {
  if (!(err instanceof ConnectError) || err.code !== Code.InvalidArgument) return err;

  if (
    input.name !== undefined &&
    roomNameCharacterCount(normalizeRoomName(input.name)) > ROOM_NAME_MAX_LENGTH
  ) {
    return new Error(`room name must be ${ROOM_NAME_MAX_LENGTH} characters or less`);
  }
  if ((input.description ?? '').length > ROOM_DESCRIPTION_MAX_LENGTH) {
    return new Error(`room description must be ${ROOM_DESCRIPTION_MAX_LENGTH} characters or less`);
  }

  return err;
}

export function createRoomCommandAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);

  return {
    async createRoom(input: {
      name: string;
      description?: string | null;
      groupId: string;
      universal?: boolean;
      threadingMode?: RoomThreadingMode;
    }): Promise<PublicRoom | null> {
      try {
        const response = await rooms.createRoom({
          name: input.name,
          description: input.description ?? '',
          groupId: input.groupId,
          universal: input.universal ?? false,
          threadingMode: input.threadingMode
        });
        return publicRoom(response.room);
      } catch (err) {
        throw roomValidationError(err, input);
      }
    },

    async updateRoom(input: {
      roomId: string;
      name?: string;
      description?: string | null;
      universal?: boolean;
      slowModeSeconds?: number;
      threadingMode?: RoomThreadingMode;
    }): Promise<PublicRoom | null> {
      try {
        const response = await rooms.updateRoom({
          roomId: input.roomId,
          name: input.name,
          description: input.description === undefined ? undefined : (input.description ?? ''),
          universal: input.universal,
          slowModeSeconds: input.slowModeSeconds,
          threadingMode: input.threadingMode,
          updateMask: updateMask(input, [
            'name',
            'description',
            'universal',
            'slowModeSeconds',
            'threadingMode'
          ])
        });
        return publicRoom(response.room);
      } catch (err) {
        throw roomValidationError(err, input);
      }
    },

    async archiveRoom(roomId: string): Promise<PublicRoom | null> {
      const response = await rooms.archiveRoom({ roomId });
      return publicRoom(response.room);
    },

    async unarchiveRoom(roomId: string): Promise<PublicRoom | null> {
      const response = await rooms.unarchiveRoom({ roomId });
      return publicRoom(response.room);
    },

    async joinRoom(roomId: string): Promise<PublicRoom | null> {
      const response = await rooms.joinRoom({ roomId });
      return publicRoom(response.room);
    },

    async startDM(participantIds: string[]): Promise<PublicRoom | null> {
      const response = await rooms.startDM({ participantIds });
      return publicRoom(response.room);
    },

    async leaveRoom(roomId: string): Promise<boolean> {
      await rooms.leaveRoom({ roomId });
      return true;
    },

    async addMember(input: { roomId: string; userId: string }): Promise<DirectoryMember | null> {
      const response = await rooms.addMember(input);
      return response.member ? mapDirectoryMember(response.member) : null;
    },

    async removeMember(input: { roomId: string; userId: string }): Promise<boolean> {
      const response = await rooms.removeMember(input);
      return response.removed;
    },

    async listSuspensions(
      input: { roomId?: string; limit?: number; offset?: number } = {},
      options: { signal?: AbortSignal } = {}
    ): Promise<RoomSuspensionList> {
      const response = await rooms.listSuspensions(
        {
          roomId: input.roomId ?? '',
          page: { limit: input.limit ?? 100, offset: input.offset ?? 0 }
        },
        { ...(options.signal ? { signal: options.signal } : {}) }
      );
      return {
        suspensions: response.suspensions.map(roomSuspension),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async joinGroup(groupId: string): Promise<string[]> {
      const response = await rooms.joinRoomGroup({ groupId });
      return response.joinedRoomIds;
    },

    async refreshTypingIndicator(
      roomId: string,
      threadRootEventId?: string | null
    ): Promise<boolean> {
      await rooms.refreshTypingIndicator({
        roomId,
        threadRootEventId: threadRootEventId ?? ''
      });
      return true;
    },

    async removeUser(input: {
      roomId: string;
      userId: string;
      reason: string;
      suspension: RoomSuspensionChoice;
    }): Promise<boolean> {
      await rooms.removeUser({
        roomId: input.roomId,
        userId: input.userId,
        reason: input.reason,
        suspension:
          input.suspension.kind === 'none'
            ? { case: undefined }
            : input.suspension.kind === 'indefinite'
              ? { case: 'suspendIndefinitely', value: true }
              : {
                  case: 'suspensionExpiresAt',
                  value: Timestamp.fromDate(new Date(input.suspension.expiresAt))
                }
      });
      return true;
    },

    async liftSuspension(input: {
      roomId: string;
      userId: string;
      reason: string;
    }): Promise<boolean> {
      await rooms.liftSuspension(input);
      return true;
    }
  };
}
