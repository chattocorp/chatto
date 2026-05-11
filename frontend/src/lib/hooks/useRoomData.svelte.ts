import { graphql } from '$lib/gql';
import { RoomType, type PresenceStatus } from '$lib/gql/graphql';
import { useReconnectTrigger } from '$lib/hooks/useReconnectCallback.svelte';
import { useConnection } from '$lib/state/server/connection.svelte';
import type { RoomMember } from '$lib/state/room';
import { untrack } from 'svelte';

export type RoomData = {
  room: { id: string; name: string; type: string };
  spaceName: string | null;
  canPostMessage: boolean;
  canPostInThread: boolean;
  canReply: boolean;
  canReplyInThread: boolean;
  canReact: boolean;
  canEditOwnMessage: boolean;
  canEditAnyMessage: boolean;
  canDeleteOwnMessage: boolean;
  canDeleteAnyMessage: boolean;
  canEchoMessage: boolean;
  canManageRoom: boolean;
  members: RoomMember[];
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
  const connection = useConnection();
  const reconnect = useReconnectTrigger();

  // undefined = loading, null = not found / no access, object = loaded
  let roomData = $state<RoomData | null | undefined>(undefined);
  let dmData = $state<DMData | null>(null);
  const roomLoadId = { current: 0 };

  // Post-PR(b) we tell channel vs DM via `Room.type` (the resolver returns
  // `RoomType.DM` for DM rooms and `CHANNEL` for everything else).
  const isDM = $derived(roomData?.room.type === RoomType.Dm);
  const isRoomLoading = $derived(roomData === undefined);

  // Load room data when roomId or reconnect changes
  $effect(() => {
    void reconnect.count;

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
              viewerCanReply
              viewerCanReplyInThread
              viewerCanReact
              viewerCanEditOwnMessage
              viewerCanEditAnyMessage
              viewerCanDeleteOwnMessage
              viewerCanDeleteAnyMessage
              viewerCanEchoMessage
              members {
                id
                login
                displayName
                avatarUrl(width: 96, height: 96)
                presenceStatus
              }
            }
            server {
              config {
                serverName
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
          spaceName: resp.data.server?.config.serverName ?? null,
          canPostMessage: resp.data.room.viewerCanPostMessage,
          canPostInThread: resp.data.room.viewerCanPostInThread,
          canReply: resp.data.room.viewerCanReply,
          canReplyInThread: resp.data.room.viewerCanReplyInThread,
          canReact: resp.data.room.viewerCanReact,
          canEditOwnMessage: resp.data.room.viewerCanEditOwnMessage,
          canEditAnyMessage: resp.data.room.viewerCanEditAnyMessage,
          canDeleteOwnMessage: resp.data.room.viewerCanDeleteOwnMessage,
          canDeleteAnyMessage: resp.data.room.viewerCanDeleteAnyMessage,
          canEchoMessage: resp.data.room.viewerCanEchoMessage,
          canManageRoom: resp.data.server?.viewerCanManageRooms ?? false,
          members: resp.data.room.members.map((m) => ({
            id: m.id,
            login: m.login,
            displayName: m.displayName,
            avatarUrl: m.avatarUrl,
            presenceStatus: m.presenceStatus
          }))
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
              members {
                id
                login
                displayName
                avatarUrl(width: 96, height: 96)
                presenceStatus
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
          participants: resp.data.room.members,
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
