import { PresenceStatus } from '$lib/chatTypes';
import { usePresenceChange, useEvent } from '$lib/hooks/useEvent.svelte';
import { createRoomMembers, type RoomMember } from '$lib/state/room';
import type { RoomData, DMData } from '$lib/hooks/useRoomData.svelte';
import { GetRoomMembersRequest } from '$lib/pb/chatto/api/v1/chat_pb';
import type { User } from '$lib/pb/chatto/core/v1/models_pb';
import { activeWireClient } from '$lib/wire';

type RoomMembersPage = {
  members: RoomMember[];
  totalCount: number;
  hasMore: boolean;
};

/**
 * Syncs room members into the shared context store.
 *
 * - Seeds from roomData/dmData when available
 * - Refetches on join/leave events
 * - Forwards presence updates
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomMembersSync(
  getProps: () => {
    roomId: string;
    isDM: boolean;
    roomData: RoomData | null | undefined;
    dmData: DMData | null;
  }
) {
  const roomMembersStore = createRoomMembers();

  async function fetchRoomMembers(offset = 0): Promise<RoomMembersPage | null> {
    const { roomId } = getProps();
    const client = activeWireClient();
    if (!client) {
      console.error('Failed to fetch room members: wire client is not connected');
      return null;
    }

    try {
      const response = await client.getRoomMembers(
        new GetRoomMembersRequest({ roomId, limit: 100, offset })
      );
      const members = response.members;
      if (!members) {
        console.error('Failed to fetch room members: missing members connection');
        return null;
      }

      return {
        members: members.users.map(wireUserToRoomMember).filter(isRoomMember),
        totalCount: members.totalCount,
        hasMore: members.hasMore
      };
    } catch (error) {
      console.error('Failed to fetch room members:', error);
      return null;
    }
  }

  async function loadMoreMembers() {
    const current = roomMembersStore.current;
    if (current.loadingMore || !current.hasMore) return;

    const currentRoomId = getProps().roomId;
    roomMembersStore.setLoadingMore(true);
    try {
      const page = await fetchRoomMembers(current.members.length);
      if (page && getProps().roomId === currentRoomId) {
        roomMembersStore.appendMembers(page.members, {
          totalCount: page.totalCount,
          hasMore: page.hasMore
        });
      }
    } finally {
      if (getProps().roomId === currentRoomId) {
        roomMembersStore.setLoadingMore(false);
      }
    }
  }

  // Seed members from roomData/dmData
  $effect(() => {
    const { isDM, dmData, roomData } = getProps();

    if (isDM && dmData) {
      roomMembersStore.setMembers(
        dmData.participants.map((p) => ({
          id: p.id,
          login: p.login,
          displayName: p.displayName,
          avatarUrl: p.avatarUrl,
          presenceStatus: p.presenceStatus
        }))
      );
    } else if (!isDM && roomData) {
      roomMembersStore.setMembers(roomData.members, {
        totalCount: roomData.membersTotalCount,
        hasMore: roomData.membersHasMore
      });
    }

    return () => {
      roomMembersStore.clear();
    };
  });

  // Refetch on membership-changing events.
  useEvent((event) => {
    if (!event.event) return;
    const eventType = event.event.__typename;
    if (
      (eventType === 'UserJoinedRoomEvent' || eventType === 'UserLeftRoomEvent') &&
      event.event.roomId === getProps().roomId
    ) {
      const currentRoomId = getProps().roomId;
      fetchRoomMembers().then((page) => {
        if (page && getProps().roomId === currentRoomId) {
          roomMembersStore.setMembers(page.members, {
            totalCount: page.totalCount,
            hasMore: page.hasMore
          });
        }
      });
    }
  });

  // Forward presence updates
  usePresenceChange((userId, status) => {
    roomMembersStore.updatePresence(userId, status);
  });

  return {
    ...roomMembersStore,
    loadMoreMembers
  };
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
