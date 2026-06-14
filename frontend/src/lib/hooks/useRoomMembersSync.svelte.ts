import { graphql, useFragment } from '$lib/gql';
import { UserAvatarUserFragmentDoc } from '$lib/gql/graphql';
import { usePresenceChange, useEvent } from '$lib/hooks/useEvent.svelte';
import { useConnection } from '$lib/state/server/connection.svelte';
import {
  createRoomMembers,
  type RoomMember
} from '$lib/state/room';
import type { RoomData, DMData } from '$lib/hooks/useRoomData.svelte';

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
  const connection = useConnection();
  const roomMembersStore = createRoomMembers();

  async function fetchRoomMembers(offset = 0): Promise<RoomMembersPage | null> {
    const { roomId } = getProps();
    const resp = await connection().client.query(
      graphql(`
        query GetRoomMembersForStore($roomId: ID!, $offset: Int) {
          room(roomId: $roomId) {
            members(limit: 100, offset: $offset) {
              users {
                ...UserAvatarUser
              }
              totalCount
              hasMore
            }
          }
        }
      `),
      { roomId, offset }
    );

    if (resp.error) {
      console.error('Failed to fetch room members:', resp.error);
      return null;
    }

    const connectionData = resp.data?.room?.members;
    if (!connectionData) {
      console.error('Failed to fetch room members: missing members connection');
      return null;
    }

    return {
      members: connectionData.users.map((m) => useFragment(UserAvatarUserFragmentDoc, m)),
      totalCount: connectionData.totalCount,
      hasMore: connectionData.hasMore
    };
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
