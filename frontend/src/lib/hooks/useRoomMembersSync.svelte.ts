import { graphql } from '$lib/gql';
import { usePresenceChange, useServerEvent } from '$lib/hooks/useServerEvent.svelte';
import { useConnection } from '$lib/state/server/connection.svelte';
import {
  createRoomMembers,
  type RoomMember
} from '$lib/state/room';
import type { RoomData, DMData } from '$lib/hooks/useRoomData.svelte';

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

  async function fetchRoomMembers(): Promise<RoomMember[]> {
    const { roomId } = getProps();
    const resp = await connection().client.query(
      graphql(`
        query GetRoomMembersForStore($roomId: ID!) {
          room(roomId: $roomId) {
            members {
              id
              login
              displayName
              avatarUrl(width: 96, height: 96)
              presenceStatus
            }
          }
        }
      `),
      { roomId }
    );

    if (resp.error) {
      console.error('Failed to fetch room members:', resp.error);
    }

    return (
      resp.data?.room?.members.map((m) => ({
        id: m.id,
        login: m.login,
        displayName: m.displayName,
        avatarUrl: m.avatarUrl,
        presenceStatus: m.presenceStatus
      })) ?? []
    );
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
      roomMembersStore.setMembers(roomData.members);
    }

    return () => {
      roomMembersStore.clear();
    };
  });

  // Refetch on join/leave events
  useServerEvent((event) => {
    if (!event.event) return;
    const eventType = event.event.__typename;
    if (
      (eventType === 'UserJoinedRoomEvent' || eventType === 'UserLeftRoomEvent') &&
      event.event.roomId === getProps().roomId
    ) {
      const currentRoomId = getProps().roomId;
      fetchRoomMembers().then((members) => {
        if (getProps().roomId === currentRoomId) {
          roomMembersStore.setMembers(members);
        }
      });
    }
  });

  // Forward presence updates
  usePresenceChange((userId, status) => {
    roomMembersStore.updatePresence(userId, status);
  });

  return roomMembersStore;
}
