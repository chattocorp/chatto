import {
  authHeaders,
  Code,
  ConnectError,
  createChattoClient,
  type ConnectAPIConfig,
} from "./connect.js";
import {
  RoomMemberService,
  ServerMemberService,
} from "@chatto/api-types/api/v1/member_directory_connect";
import type { DirectoryMember as APIDirectoryMember } from "@chatto/api-types/api/v1/member_directory_pb";
import { PresenceStatus as APIPresenceStatus } from "@chatto/api-types/api/v1/presence_pb";
import { PresenceStatus } from "./renderTypes.js";

export type MemberDirectoryAPIConfig = ConnectAPIConfig;

export type DirectoryMember = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl: string | null;
  presenceStatus: PresenceStatus;
  customStatus: {
    emoji: string;
    text: string;
    expiresAt: string | null;
  } | null;
  roles: string[];
  createdAt: string | null;
};

export type MemberDirectoryPage = {
  members: DirectoryMember[];
  totalCount: number;
  hasMore: boolean;
};

export function createMemberDirectoryAPI(config: MemberDirectoryAPIConfig) {
  const serverMembers = createChattoClient(ServerMemberService, config);
  const roomMembers = createChattoClient(RoomMemberService, config);
  const headers = () => authHeaders(config);

  return {
    async listServerMembers(
      search = "",
      limit = 20,
      offset = 0,
    ): Promise<MemberDirectoryPage> {
      const response = await serverMembers.listMembers(
        { search, page: { limit, offset } },
        { headers: headers() },
      );
      return {
        members: response.members.map(mapDirectoryMember),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false,
      };
    },

    async getServerMember(userId: string): Promise<DirectoryMember | null> {
      try {
        const response = await serverMembers.getMember(
          { userId },
          { headers: headers() },
        );
        return response.member ? mapDirectoryMember(response.member) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetServerMembers(userIds: string[]): Promise<DirectoryMember[]> {
      const response = await serverMembers.batchGetMembers(
        { userIds },
        { headers: headers() },
      );
      return response.members.map(mapDirectoryMember);
    },

    async listRoomMembers(
      roomId: string,
      search = "",
      limit = 20,
      offset = 0,
    ): Promise<MemberDirectoryPage> {
      const response = await roomMembers.listMembers(
        { roomId, search, page: { limit, offset } },
        { headers: headers() },
      );
      return {
        members: response.members.map(mapDirectoryMember),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false,
      };
    },

    async getRoomMember(
      roomId: string,
      userId: string,
    ): Promise<DirectoryMember | null> {
      try {
        const response = await roomMembers.getMember(
          { roomId, userId },
          { headers: headers() },
        );
        return response.member ? mapDirectoryMember(response.member) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetRoomMembers(
      roomId: string,
      userIds: string[],
    ): Promise<DirectoryMember[]> {
      const response = await roomMembers.batchGetMembers(
        { roomId, userIds },
        { headers: headers() },
      );
      return response.members.map(mapDirectoryMember);
    },
  };
}

export type MemberDirectoryAPI = ReturnType<typeof createMemberDirectoryAPI>;

export function mapDirectoryMember(
  member: APIDirectoryMember,
): DirectoryMember {
  const profile = member.profile;
  const summary = profile?.user;
  return {
    id: summary?.id ?? "",
    login: summary?.login ?? "",
    displayName: summary?.displayName ?? "",
    deleted: summary?.deleted ?? false,
    avatarUrl: summary?.avatarUrl ?? null,
    presenceStatus: apiPresenceStatus(
      profile?.presenceStatus ?? APIPresenceStatus.UNSPECIFIED,
    ),
    customStatus: profile?.customStatus
      ? {
          emoji: profile.customStatus.emoji,
          text: profile.customStatus.text,
          expiresAt:
            profile.customStatus.expiresAt?.toDate().toISOString() ?? null,
        }
      : null,
    roles: [...member.roles],
    createdAt: member.createdAt?.toDate().toISOString() ?? null,
  };
}

function apiPresenceStatus(status: APIPresenceStatus): PresenceStatus {
  switch (status) {
    case APIPresenceStatus.AWAY:
      return PresenceStatus.Away;
    case APIPresenceStatus.DO_NOT_DISTURB:
      return PresenceStatus.DoNotDisturb;
    case APIPresenceStatus.ONLINE:
      return PresenceStatus.Online;
    case APIPresenceStatus.OFFLINE:
    case APIPresenceStatus.UNSPECIFIED:
    default:
      return PresenceStatus.Offline;
  }
}
