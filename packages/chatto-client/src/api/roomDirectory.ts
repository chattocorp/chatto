import { listAllDirectoryRooms } from './roomPages.js';
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import type {
  RoomGroup,
  RoomGroupItem,
  RoomGroupViewerState,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomArchiveFilter, RoomDirectoryScope } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { normalizeRoomThreadingMode, type RoomThreadingMode } from '../util/roomThreading.js';

/** Pending invitation metadata does not grant room-content access. */
export type ViewerRoomInvitation = {
  id: string;
  senderId: string;
  expiresAt: Date;
  viewerCanAccept: boolean;
};

export type DirectoryRoomSummary = {
  /** Explicit notice-management grant for a current member of an active channel. */
  canManageNotice?: boolean;
  invitation?: ViewerRoomInvitation;
  canInvite?: boolean;
  id: string;
  name: string;
  description: string | null;
  /** Original room creation time; absent on older servers. */
  createdAt?: string | null;
  kind: RoomKind;
  archived: boolean;
  isUniversal: boolean;
  slowModeSeconds: number;
  /** Undefined for the legacy shared cooldown; zero disables thread cooldowns. */
  threadSlowModeSeconds?: number;
  threadingMode: RoomThreadingMode;
  /** Absent on older servers; clients then show join/leave activity. */
  showMembershipEvents?: boolean;
  slowModeNextPostAt: string | null;
  isMember: boolean;
  hasUnread: boolean;
  /** Latest ordinary DM post, including replies; null when unavailable. */
  lastMessageAt?: string | null;
  /** Latest ordinary DM root post; null when unavailable. */
  lastRootMessageAt?: string | null;
  canReadMessages: boolean | null;
  canJoinRoom: boolean;
  /** Joining authority without an invitation, including current members; absent on older servers. */
  canRejoin?: boolean;
  canManageRoom: boolean;
  /** Normal navigation visibility. False can accompany management-only metadata.
   * Absent means an older server did not report a decision. */
  canListRoom?: boolean;
};

export type DirectoryRoomDetails = DirectoryRoomSummary & {
  /** True only when the server explicitly grants interaction-only message access. */
  hasLimitedMessageAccess: boolean;
  canPostMessage: boolean;
  canPostInThread: boolean;
  canAttach: boolean;
  canReact: boolean;
  canEchoMessage: boolean;
  /** Server-authorized pin mutations; pinning also requires access to the message. */
  canPinMessages: boolean;
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
  description: string;
  canCreateRoom: boolean;
  canManageGroup: boolean;
  roomIds: string[];
  items: DirectoryRoomGroupItem[];
};

export { RoomDirectoryScope };
export { RoomArchiveFilter };
export { RoomKind };

const RoomPermission = {
  Attach: 'message.attach',
  BanMember: 'room.remove-member',
  CreateRoom: 'room.create',
  EchoMessage: 'message.echo',
  PinMessages: 'message.pin',
  JoinRoom: 'room.join',
  ListRoom: 'room.list',
  ManageMessage: 'message.manage',
  ManageRoom: 'room.manage',
  ReadInteractions: 'message.read-interactions',
  ReadMessages: 'message.read',
  PostInThread: 'message.post-in-thread',
  PostMessage: 'message.post',
  React: 'message.react'
} as const;

/** Read options shared by list, get, and batch group reads. Archive selection
 * never grants visibility; current server permissions determine returned entries. */
export type RoomGroupReadOptions = { archiveFilter?: RoomArchiveFilter; signal?: AbortSignal };

export function createRoomDirectoryAPI(config: ConnectAPIConfig) {
  const directory = createChattoClient(RoomDirectoryService, config);

  return {
    async listRooms(
      scope: RoomDirectoryScope,
      options: { signal?: AbortSignal } = {}
    ): Promise<DirectoryRoomSummary[]> {
      const rooms = await listAllDirectoryRooms((page) =>
        directory.listRooms({ scope, page }, { signal: options.signal })
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

    /** Read groups in configured order. ALL includes archived entries the caller can see or manage. */
    async listRoomGroups(options: RoomGroupReadOptions = {}): Promise<DirectoryRoomGroup[]> {
      const response = await directory.listRoomGroups(
        { archiveFilter: options.archiveFilter },
        { signal: options.signal }
      );
      return response.groups.map(mapRoomGroup);
    },

    /** Read one group with the same archive and permission rules as listRoomGroups. */
    async getRoomGroup(
      groupId: string,
      options: RoomGroupReadOptions = {}
    ): Promise<DirectoryRoomGroup | null> {
      try {
        const response = await directory.getRoomGroup(
          { groupId, archiveFilter: options.archiveFilter },
          { signal: options.signal }
        );
        return response.group ? mapRoomGroup(response.group) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    /** Read up to 100 groups in first-seen order, omitting unknown IDs. */
    async batchGetRoomGroups(
      groupIds: string[],
      options: RoomGroupReadOptions = {}
    ): Promise<DirectoryRoomGroup[]> {
      const response = await directory.batchGetRoomGroups(
        { groupIds, archiveFilter: options.archiveFilter },
        { signal: options.signal }
      );
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
    canAttach: hasRoomPermission(entry.viewerState, RoomPermission.Attach),
    canReact: hasRoomPermission(entry.viewerState, RoomPermission.React),
    canEchoMessage: hasRoomPermission(entry.viewerState, RoomPermission.EchoMessage),
    canPinMessages: hasRoomPermission(entry.viewerState, RoomPermission.PinMessages),
    canManageOthersMessage: hasRoomPermission(entry.viewerState, RoomPermission.ManageMessage),
    canManageRoom: hasRoomPermission(entry.viewerState, RoomPermission.ManageRoom),
    canBanRoomMembers: hasRoomPermission(entry.viewerState, RoomPermission.BanMember)
  };
}

export function mapDirectoryRoom(entry: RoomWithViewerState): DirectoryRoomSummary | null {
  if (!entry.room) return null;
  return {
    invitation: entry.viewerState?.invitation?.expiresAt
      ? {
          id: entry.viewerState.invitation.id,
          senderId: entry.viewerState.invitation.senderId,
          expiresAt: entry.viewerState.invitation.expiresAt.toDate(),
          viewerCanAccept: entry.viewerState.invitation.canAccept
        }
      : undefined,
    canInvite:
      entry.viewerState?.permissions.some((p) => p.permission === 'room.invite' && p.granted) ??
      false,
    canManageNotice: hasRoomPermission(entry.viewerState, 'room.manage-notice'),
    id: entry.room.id,
    name: entry.room.name,
    description: entry.room.description || null,
    createdAt: entry.room.createdAt?.toDate().toISOString() ?? null,
    kind: entry.room.kind,
    archived: entry.room.archived,
    isUniversal: entry.room.universal,
    slowModeSeconds: entry.room.slowModeSeconds ?? 0,
    threadSlowModeSeconds: entry.room.threadSlowModeSeconds,
    threadingMode: normalizeRoomThreadingMode(entry.room.kind, entry.room.threadingMode),
    showMembershipEvents: entry.room.showMembershipEvents,
    slowModeNextPostAt: entry.viewerState?.slowModeNextPostAt?.toDate().toISOString() ?? null,
    isMember: entry.viewerState?.isMember ?? false,
    hasUnread: entry.viewerState?.hasUnread ?? false,
    lastMessageAt: entry.lastMessageAt?.toDate().toISOString() ?? null,
    lastRootMessageAt: entry.lastRootMessageAt?.toDate().toISOString() ?? null,
    canReadMessages: anyRoomPermissionDecision(entry.viewerState, [
      RoomPermission.ReadMessages,
      RoomPermission.ReadInteractions
    ]),
    canJoinRoom: hasRoomPermission(entry.viewerState, RoomPermission.JoinRoom),
    canRejoin: entry.viewerState?.canRejoin,
    canListRoom: roomPermissionDecision(entry.viewerState, RoomPermission.ListRoom) ?? undefined,
    canManageRoom: hasRoomPermission(entry.viewerState, RoomPermission.ManageRoom)
  };
}

export function mapRoomGroup(group: RoomGroup): DirectoryRoomGroup {
  return {
    id: group.id,
    name: group.name,
    description: group.description,
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
