import { graphql } from '$lib/gql';
import type { PresenceStatus } from '$lib/gql/graphql';
import { useReconnectTrigger } from '$lib/hooks/useReconnectCallback.svelte';
import { useConnection } from '$lib/state/instance/connection.svelte';
import type { RoomMember } from '$lib/state/room';
import { DM_SPACE_ID } from '$lib/constants';
import { untrack } from 'svelte';

export type RoomData = {
  room: { id: string; name: string };
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
 * Delays (ms) for retrying the room query on `networkError` during the *initial*
 * load. After all retries are exhausted, `roomData` stays `undefined` and the
 * blank-screen condition is logged at error level. We deliberately don't flip
 * to `null` because that would trigger the "not found" redirect path in
 * `Room.svelte` and clobber `lastRoom`.
 */
const ROOM_QUERY_RETRY_DELAYS_MS = [1000, 3000, 8000];

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
export function useRoomData(getProps: () => { spaceId: string; roomId: string }) {
  const connection = useConnection();
  const reconnect = useReconnectTrigger();

  // undefined = loading, null = not found / no access, object = loaded
  let roomData = $state<RoomData | null | undefined>(undefined);
  let dmData = $state<DMData | null>(null);
  const roomLoadId = { current: 0 };

  const isDM = $derived(getProps().spaceId === DM_SPACE_ID);
  const isRoomLoading = $derived(roomData === undefined);

  // Load room data when roomId, spaceId, or reconnect changes
  $effect(() => {
    void reconnect.count;

    const { spaceId, roomId } = getProps();
    const thisLoadId = ++roomLoadId.current;
    const currentSpaceId = spaceId;
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

    function runQuery(attempt: number) {
      connection()
        .client.query(
          graphql(`
            query GetRoom($spaceId: ID!, $roomId: ID!) {
              room(spaceId: $spaceId, roomId: $roomId) {
                id
                name
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
              space(id: $spaceId) {
                id
                name
                viewerCanManageRooms
              }
            }
          `),
          { spaceId: currentSpaceId, roomId: currentRoomId }
        )
        .toPromise()
        .then((resp) => {
          if (roomLoadId.current !== thisLoadId) return;

          if (resp.error?.networkError) {
            // Reconnect / wake-from-sleep with prior data visible: preserve
            // it and rely on the WS reconnect handler to refresh later.
            // Initial load has nothing to preserve, so retry with backoff —
            // otherwise the user is stuck on a blank screen forever.
            const hasPriorData = untrack(() => roomData) !== undefined;
            if (hasPriorData) {
              console.warn('[useRoomData] networkError on refetch, keeping prior data', resp.error);
              return;
            }
            if (attempt < ROOM_QUERY_RETRY_DELAYS_MS.length) {
              const delay = ROOM_QUERY_RETRY_DELAYS_MS[attempt];
              console.warn(
                `[useRoomData] networkError on initial load, retrying in ${delay}ms (attempt ${attempt + 1}/${ROOM_QUERY_RETRY_DELAYS_MS.length})`,
                resp.error
              );
              setTimeout(() => {
                if (roomLoadId.current !== thisLoadId) return;
                runQuery(attempt + 1);
              }, delay);
              return;
            }
            console.error(
              '[useRoomData] networkError on initial load, retries exhausted; roomData stays undefined',
              resp.error
            );
            return;
          }

          if (!resp.data?.room) {
            roomData = null;
            return;
          }

          roomData = {
            room: resp.data.room,
            spaceName: resp.data.space?.name ?? null,
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
            canManageRoom: resp.data.space?.viewerCanManageRooms ?? false,
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
    }

    runQuery(0);
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
          query GetDMRoomMembers($spaceId: ID!, $roomId: ID!) {
            room(spaceId: $spaceId, roomId: $roomId) {
              id
              members {
                id
                login
                displayName
                avatarUrl(width: 96, height: 96)
                presenceStatus
              }
            }
            me {
              id
            }
          }
        `),
        { spaceId: DM_SPACE_ID, roomId: getProps().roomId }
      )
      .toPromise()
      .then((resp) => {
        if (!resp.data?.room) {
          dmData = { participants: [], currentUserId: null };
          return;
        }
        dmData = {
          participants: resp.data.room.members,
          currentUserId: resp.data.me?.id ?? null
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
