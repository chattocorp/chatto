import type { DirectoryMember } from '$lib/api-client/memberDirectory';
import { mapDirectoryRoomDetails, RoomKind } from '$lib/api-client/roomDirectory';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

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
 * Select room metadata and complete membership from the retained server
 * projection. Room switches are synchronous once the server has hydrated.
 *
 * `undefined` means the server projection is genuinely cold, `null` means a
 * ready projection contains no visible room, and an object is renderable data.
 */
export function useRoomData(getProps: () => { roomId: string }) {
  // The registry is keyed by the frontend registration ID (the URL segment),
  // not by the backend identity advertised through ServerConnection.
  const store = $derived(serverRegistry.tryGetStore(getActiveServer()));

  const roomData = $derived.by<RoomData | null | undefined>(() => {
    const currentStore = store;
    if (!currentStore?.realtimeSync.hasUsableProjection) return undefined;
    const projectedRoom = currentStore.projection.rooms.get(getProps().roomId)?.room;
    const room = mapDirectoryRoomDetails(projectedRoom);
    // A stale projection can render known rooms immediately, but absence is
    // not authoritative until the activation catch-up reaches caught_up.
    if (!room) return currentStore.realtimeSync.phase === 'ready' ? null : undefined;
    return {
      room: {
        id: room.id,
        name: room.name,
        description: room.description,
        type: room.kind,
        isUniversal: room.isUniversal
      },
      spaceName: currentStore.serverInfo.name ?? null,
      canPostMessage: room.canPostMessage,
      canPostInThread: room.canPostInThread,
      canAttach: room.canAttach,
      canReact: room.canReact,
      canManageOthersMessage: room.canManageOthersMessage,
      canEchoMessage: room.canEchoMessage,
      canManageRoom: room.canManageRoom,
      canBanRoomMembers: room.canBanRoomMembers
    };
  });

  const isDM = $derived(roomData?.room.type === RoomKind.DM);
  const dmData = $derived.by<DMData | null>(() => {
    const currentStore = store;
    if (!isDM || !currentStore?.realtimeSync.hasUsableProjection) return null;
    return {
      participants: currentStore.projectedMembersForRoom(getProps().roomId),
      currentUserId: currentStore.currentUser.user?.id ?? null
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
      return roomData === undefined;
    }
  };
}
