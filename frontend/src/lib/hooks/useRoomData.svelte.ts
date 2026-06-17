import { graphql, useFragment } from '$lib/gql';
import { RoomType, UserAvatarUserFragmentDoc, type PresenceStatus } from '$lib/gql/graphql';
import { useActiveRoomLayoutUpdated } from '$lib/hooks/useEvent.svelte';
import { useReconnectTrigger } from '$lib/hooks/useReconnectCallback.svelte';
import { useConnection } from '$lib/state/server/connection.svelte';
import type { RoomMember } from '$lib/state/room';
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
  members: RoomMember[];
  membersTotalCount: number;
  membersHasMore: boolean;
};

export type DMData = {
  participants: Array<{
    id: string;
    login: string;
    displayName: string;
    deleted?: boolean;
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
  const connection = useConnection();
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

    connection()
      .client.query(
        graphql(`
          query GetRoom($roomId: ID!) {
            room(roomId: $roomId) {
              id
              name
              type
              viewerCanPostMessage
              viewerCanPostInThread
              viewerCanReact
              viewerCanManageOthersMessage
              viewerCanEchoMessage
              viewerCanManageRoom
              viewerCanBanRoomMembers
              members(limit: 100) {
                users {
                  ...UserAvatarUser
                }
                totalCount
                hasMore
              }
            }
            server {
              profile {
                name
              }
              viewerCanManageRooms
            }
          }
        `),
        { roomId: currentRoomId }
      )
      .toPromise()
      .then((resp) => {
        if (roomLoadId.current !== thisLoadId) return;

        // Transient network failure (e.g., wake-from-sleep) — keep prior data
        // visible and let the reconnect handler retry. Don't flip to null,
        // which would trigger the not-found redirect path.
        // Logged so a stuck-blank-screen incident leaves a fingerprint.
        if (resp.error?.networkError) {
          console.warn(
            '[useRoomData] networkError, ignoring (roomData stays at prior value)',
            { roomId: currentRoomId, error: resp.error }
          );
          return;
        }

        if (!resp.data?.room) {
          roomData = null;
          return;
        }

        roomData = {
          room: resp.data.room,
          spaceName: resp.data.server?.profile.name ?? null,
          canPostMessage: resp.data.room.viewerCanPostMessage,
          canPostInThread: resp.data.room.viewerCanPostInThread,
          canReact: resp.data.room.viewerCanReact,
          canManageOthersMessage: resp.data.room.viewerCanManageOthersMessage,
          canEchoMessage: resp.data.room.viewerCanEchoMessage,
          canManageRoom: resp.data.room.viewerCanManageRoom,
          canBanRoomMembers: resp.data.room.viewerCanBanRoomMembers,
          members: resp.data.room.members.users.map((m) => useFragment(UserAvatarUserFragmentDoc, m)),
          membersTotalCount: resp.data.room.members.totalCount,
          membersHasMore: resp.data.room.members.hasMore
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

    void reconnect.count;

    connection()
      .client.query(
        graphql(`
          query GetDMRoomMembers($roomId: ID!) {
            room(roomId: $roomId) {
              id
              members(limit: 100) {
                users {
                  ...UserAvatarUser
                }
                totalCount
                hasMore
              }
            }
            viewer {
              user {
                id
              }
            }
          }
        `),
        { roomId: getProps().roomId }
      )
      .toPromise()
      .then((resp) => {
        if (!resp.data?.room) {
          dmData = { participants: [], currentUserId: null };
          return;
        }
        dmData = {
          participants: resp.data.room.members.users.map((m) =>
            useFragment(UserAvatarUserFragmentDoc, m)
          ),
          currentUserId: resp.data.viewer?.user.id ?? null
        };
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
