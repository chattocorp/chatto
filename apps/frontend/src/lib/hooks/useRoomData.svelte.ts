import type { DirectoryMember } from '$lib/api-client/memberDirectory';
import { mapDirectoryRoomDetails, RoomKind } from '$lib/api-client/roomDirectory';
import { RoomThreadingMode } from '$lib/roomThreading';
import { roomKindOrChannel } from '$lib/api-client/enumDefaults';
import { useServerScope } from '$lib/state/server/scope.svelte';

export type RoomData = {
  room: {
    id: string;
    name: string;
    type: RoomKind;
    description?: string | null;
    isUniversal: boolean;
    slowModeSeconds: number;
    threadingMode: RoomThreadingMode;
    archived?: boolean;
  };
  spaceName: string | null;
  canReadMessages: boolean | null;
  hasLimitedMessageAccess: boolean;
  canPostMessage: boolean;
  canPostInThread: boolean;
  canPostInteractions: boolean;
  canAttach: boolean;
  canReact: boolean;
  canManageOthersMessage: boolean;
  canEchoMessage: boolean;
  canManageRoom: boolean;
  canBanRoomMembers: boolean;
  slowModeNextPostAt: string | null;
};

export type DMData = {
  /** Stable member IDs from the room projection, including unresolved users. */
  participantIds: string[];
  participants: Array<{
    id: string;
    login: string;
    displayName: string;
    isBot?: boolean;
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
  const serverScope = useServerScope();
  const store = $derived(serverScope.store);

  const roomData = $derived.by<RoomData | null | undefined>(() => {
    const currentStore = store;
    if (!currentStore.realtimeSync.hasDisplayableView) return undefined;
    const projectedRoom = currentStore.projection.rooms.get(getProps().roomId);
    const live = mapDirectoryRoomDetails(projectedRoom);
    const saved = currentStore.savedRooms.find((room) => room.id === getProps().roomId);
    const room = live ?? saved;
    // A stale projection can render known rooms immediately, but absence is
    // not authoritative until the activation catch-up reaches caught_up.
    if (!room) return currentStore.realtimeSync.phase === 'ready' ? null : undefined;
    const canAct = currentStore.isAuthenticated;
    return {
      room: {
        id: room.id,
        name: room.name,
        description: live?.description,
        type: roomKindOrChannel(room.kind ?? RoomKind.CHANNEL),
        isUniversal: live?.isUniversal ?? saved?.universal ?? false,
        slowModeSeconds: live?.slowModeSeconds ?? 0,
        threadingMode: live?.threadingMode ?? RoomThreadingMode.DISABLED,
        archived: live?.archived
      },
      spaceName: currentStore.serverInfo.name ?? null,
      canReadMessages: live?.canReadMessages ?? null,
      hasLimitedMessageAccess: live?.hasLimitedMessageAccess ?? false,
      canPostMessage: canAct && (live?.canPostMessage ?? false),
      canPostInThread: canAct && (live?.canPostInThread ?? false),
      canPostInteractions: canAct && (live?.canPostInteractions ?? false),
      canAttach: canAct && (live?.canAttach ?? false),
      canReact: canAct && (live?.canReact ?? false),
      canManageOthersMessage: canAct && (live?.canManageOthersMessage ?? false),
      canEchoMessage: canAct && (live?.canEchoMessage ?? false),
      canManageRoom: canAct && (live?.canManageRoom ?? false),
      canBanRoomMembers: canAct && (live?.canBanRoomMembers ?? false),
      slowModeNextPostAt: live?.slowModeNextPostAt ?? null
    };
  });

  const isDM = $derived(roomData?.room.type === RoomKind.DM);
  const dmData = $derived.by<DMData | null>(() => {
    const currentStore = store;
    if (!isDM || !currentStore.realtimeSync.hasDisplayableView) return null;
    const projectedRoom = currentStore.projection.rooms.get(getProps().roomId);
    return {
      participantIds: projectedRoom?.memberUserIds ?? [],
      participants: currentStore.projectedMembersForRoom(getProps().roomId),
      currentUserId: currentStore.currentUser.user?.id ?? currentStore.savedView?.userId ?? null
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
