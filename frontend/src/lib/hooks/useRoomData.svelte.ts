import { RoomType, PresenceStatus } from '$lib/chatTypes';
import { useActiveRoomLayoutUpdated } from '$lib/hooks/useEvent.svelte';
import { useReconnectTrigger } from '$lib/hooks/useReconnectCallback.svelte';
import type { MentionRole, RoomMember } from '$lib/state/room';
import { activeWireClient } from '$lib/wire';
import { GetRoomRequest, type GetRoomResponse } from '$lib/pb/chatto/api/v1/chat_pb';
import { RoomKind, type User } from '$lib/pb/chatto/core/v1/models_pb';
import { untrack } from 'svelte';

export type RoomData = {
  room: { id: string; name: string; type: string };
  spaceName: string | null;
  canPostMessage: boolean;
  canPostInThread: boolean;
  canReact: boolean;
  canManageOthersMessage: boolean;
  canEchoMessage: boolean;
  canManageRoom: boolean;
  canBanRoomMembers: boolean;
  viewerUserId: string | null;
  mentionRoles: MentionRole[];
  members: RoomMember[];
  membersTotalCount: number;
  membersHasMore: boolean;
};

export type DMData = {
  participants: Array<{
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    presenceStatus: PresenceStatus;
  }>;
  currentUserId: string | null;
};

/**
 * Loads room metadata and DM participant data.
 *
 * Returns reactive state that updates when room/space changes or WebSocket reconnects.
 * The three-state pattern for roomData:
 * - `undefined` = loading (initial)
 * - `null` = not found / no access
 * - `object` = loaded
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomData(getProps: () => { roomId: string }) {
  const reconnect = useReconnectTrigger();

  // Refresh on room-groups-updated too: an admin renaming/reordering
  // groups, moving rooms between groups, or editing per-group / per-room
  // permissions can change any viewerCan* permission for this room.
  // Bump a counter and let the loading effect react.
  let layoutTrigger = $state(0);
  useActiveRoomLayoutUpdated(() => {
    layoutTrigger++;
  });

  // undefined = loading, null = not found / no access, object = loaded
  let roomData = $state<RoomData | null | undefined>(undefined);
  let dmData = $state<DMData | null>(null);
  const roomLoadId = { current: 0 };

  // Post-PR(b) we tell channel vs DM via `Room.type` (the resolver returns
  // `RoomType.DM` for DM rooms and `CHANNEL` for everything else).
  const isDM = $derived(roomData?.room.type === RoomType.Dm);
  const isRoomLoading = $derived(roomData === undefined);

  // Load room data when roomId, reconnect, or the room-sets layout changes
  $effect(() => {
    void reconnect.count;
    void layoutTrigger;

    const { roomId } = getProps();
    const thisLoadId = ++roomLoadId.current;
    const currentRoomId = roomId;

    // Don't reset roomData to undefined when staying in the same room (reconnect case).
    untrack(() => {
      const currentRoom = roomData;
      if (currentRoom && currentRoom.room.id === currentRoomId) {
        // Same room, just reconnecting — keep existing data visible while refetching
      } else {
        roomData = undefined;
      }
    });

    fetchWireRoom(currentRoomId)
      .then((resp) => {
        if (roomLoadId.current !== thisLoadId) return;

        if (!resp.room?.id) {
          roomData = null;
          return;
        }

        roomData = {
          room: {
            id: resp.room.id,
            name: resp.room.name,
            type: roomTypeFromWire(resp.room.kind)
          },
          spaceName: resp.serverName || null,
          canPostMessage: resp.viewerCanPostMessage,
          canPostInThread: resp.viewerCanPostInThread,
          canReact: resp.viewerCanReact,
          canManageOthersMessage: resp.viewerCanManageOthersMessage,
          canEchoMessage: resp.viewerCanEchoMessage,
          canManageRoom: resp.viewerCanManageRoom,
          canBanRoomMembers: resp.viewerCanBanRoomMembers,
          viewerUserId: resp.viewerUserId || null,
          mentionRoles: resp.mentionRoles.map((role) => ({
            name: role.name,
            isSystem: role.isSystem,
            position: role.position,
            pingable: role.pingable
          })),
          members: resp.members?.users.map(wireUserToRoomMember).filter(isRoomMember) ?? [],
          membersTotalCount: resp.members?.totalCount ?? 0,
          membersHasMore: resp.members?.hasMore ?? false
        };
      })
      .catch((err) => {
        if (roomLoadId.current !== thisLoadId) return;
        console.error('Failed to load room:', err);
        roomData = null;
      });
  });

  // Load DM participants
  $effect(() => {
    if (!isDM) {
      dmData = null;
      return;
    }

    dmData = {
      participants: roomData?.members ?? [],
      currentUserId: roomData?.viewerUserId ?? null
    };
  });

  return {
    get roomData() {
      return roomData;
    },
    get dmData() {
      return dmData;
    },
    get isDM() {
      return isDM;
    },
    get isRoomLoading() {
      return isRoomLoading;
    }
  };
}

async function fetchWireRoom(roomId: string): Promise<GetRoomResponse> {
  const client = activeWireClient();
  if (!client) throw new Error('wire client is not connected');
  return client.getRoom(new GetRoomRequest({ roomId, membersLimit: 100 }));
}

function roomTypeFromWire(kind: RoomKind): RoomType {
  return kind === RoomKind.DM ? RoomType.Dm : RoomType.Channel;
}

function wireUserToRoomMember(user: User | undefined): RoomMember | null {
  if (!user?.id) return null;
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

function isRoomMember(member: RoomMember | null): member is RoomMember {
  return member !== null;
}
