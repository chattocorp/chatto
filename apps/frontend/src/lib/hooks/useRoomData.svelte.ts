import { Code, ConnectError } from '@connectrpc/connect';
import { createMemberDirectoryAPI, type DirectoryMember } from '@chatto/api-client/memberDirectory';
import { createRoomDirectoryAPI, RoomKind } from '@chatto/api-client/roomDirectory';
import { useActiveRoomLayoutUpdated } from '$lib/hooks/useEvent.svelte';
import { useReconnectTrigger } from '$lib/hooks/useReconnectCallback.svelte';
import { useConnection } from '$lib/state/server/connection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { untrack } from 'svelte';

export type RoomData = {
  room: {
    id: string;
    name: string;
    type: RoomKind;
    description?: string | null;
    isUniversal: boolean;
  };
  spaceName: string | null;
  canPostMessage: boolean;
  canPostInThread: boolean;
  canAttach: boolean;
  canReact: boolean;
  canManageOthersMessage: boolean;
  canEchoMessage: boolean;
  canManageRoom: boolean;
  canBanRoomMembers: boolean;
};

export type DMData = {
  participants: Array<{
    id: string;
    login: string;
    displayName: string;
    deleted?: boolean;
    avatarUrl?: string | null;
    presenceStatus: DirectoryMember['presenceStatus'];
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
  const connection = useConnection();
  const reconnect = useReconnectTrigger();

  // Refresh on room-groups-updated too: an admin renaming/reordering
  // groups, moving rooms between groups, or editing per-group / per-room
  // permissions can change any viewerCan* permission for this room.
  // Bump a counter and let the loading effect react.
  let layoutTrigger = $state(0);
  useActiveRoomLayoutUpdated((info) => {
    const { roomId } = getProps();
    if (info.roomId && info.roomId !== roomId) return;
    layoutTrigger++;
  });

  // undefined = loading, null = not found / no access, object = loaded
  let roomData = $state<RoomData | null | undefined>(undefined);
  let dmData = $state<DMData | null>(null);
  const roomLoadId = { current: 0 };
  const dmLoadId = { current: 0 };

  const isDM = $derived(roomData?.room.type === RoomKind.DM);
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

    const currentConnection = connection();
    const api = createRoomDirectoryAPI({
      serverId: currentConnection.serverId,
      baseUrl: currentConnection.connectBaseUrl,
      bearerToken: currentConnection.bearerToken
    });

    api
      .getRoom(currentRoomId)
      .then((loadedRoom) => {
        if (roomLoadId.current !== thisLoadId) return;

        if (!loadedRoom) {
          roomData = null;
          return;
        }

        roomData = {
          room: {
            id: loadedRoom.id,
            name: loadedRoom.name,
            description: loadedRoom.description,
            type: loadedRoom.kind,
            isUniversal: loadedRoom.isUniversal
          },
          spaceName: serverName(currentConnection.serverId),
          canPostMessage: loadedRoom.canPostMessage,
          canPostInThread: loadedRoom.canPostInThread,
          canAttach: loadedRoom.canAttach,
          canReact: loadedRoom.canReact,
          canManageOthersMessage: loadedRoom.canManageOthersMessage,
          canEchoMessage: loadedRoom.canEchoMessage,
          canManageRoom: loadedRoom.canManageRoom,
          canBanRoomMembers: loadedRoom.canBanRoomMembers
        };
      })
      .catch((err) => {
        if (roomLoadId.current !== thisLoadId) return;
        if (isTransientRoomLoadError(err)) {
          console.warn('[useRoomData] transient room load failure, keeping prior roomData', {
            roomId: currentRoomId,
            error: err
          });
          return;
        }
        console.error('Failed to load room:', err);
        roomData = null;
      });
  });

  // Load DM participants
  $effect(() => {
    if (!isDM) {
      dmLoadId.current++;
      dmData = null;
      return;
    }

    void reconnect.count;
    const currentRoomId = getProps().roomId;
    const thisLoadId = ++dmLoadId.current;

    const currentConnection = connection();
    const api = createMemberDirectoryAPI({
      baseUrl: currentConnection.connectBaseUrl,
      bearerToken: currentConnection.bearerToken
    });
    api
      .listRoomMembers(currentRoomId, '', 100, 0)
      .then((resp) => {
        if (dmLoadId.current !== thisLoadId) return;
        dmData = {
          participants: resp.members.map((member) => ({
            id: member.id,
            login: member.login,
            displayName: member.displayName,
            deleted: member.deleted,
            avatarUrl: member.avatarUrl,
            presenceStatus: member.presenceStatus
          })),
          currentUserId: currentConnection.serverId
            ? (serverRegistry.tryGetStore(currentConnection.serverId)?.currentUser.user?.id ?? null)
            : null
        };
      })
      .catch((err) => {
        if (dmLoadId.current !== thisLoadId) return;
        console.warn('[useRoomData] failed to load DM members', {
          roomId: currentRoomId,
          error: err
        });
      });
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

function serverName(serverId: string | null | undefined): string | null {
  return serverId ? (serverRegistry.tryGetStore(serverId)?.serverInfo.name ?? null) : null;
}

function isTransientRoomLoadError(err: unknown): boolean {
  if (err instanceof ConnectError) {
    return err.code === Code.Unavailable || err.code === Code.DeadlineExceeded;
  }
  return err instanceof TypeError;
}
