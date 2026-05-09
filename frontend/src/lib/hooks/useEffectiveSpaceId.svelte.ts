import { DM_SPACE_ID } from '$lib/constants';
import { RoomType } from '$lib/gql/graphql';
import { getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
import { getSpaceRoomsStore } from '$lib/state/space';

/**
 * Resolves which underlying space a roomId actually lives in, given the
 * post-#330-phase-3 URL shape that drops the spaceId segment. The merged
 * SpaceRoomsStore (channels + the user's DMs) is the lookup source.
 *
 * Returns:
 * - `null` while the rooms store is doing its initial load — callers should
 *   gate downstream rendering on a non-null value to avoid resolving against
 *   stale defaults (a deep-link to a DM would otherwise briefly resolve to
 *   the primary space, 404, and trigger Room.svelte's not-found redirect).
 * - `DM_SPACE_ID` when the room is a DM the user has joined.
 * - The active (primary) space when the room is a channel the user has joined.
 * - `DM_SPACE_ID` as a fallback when the room isn't in the store at all —
 *   typically a freshly-created DM that hasn't been seeded with its first
 *   message (ListDMConversations filters empty rooms). If the room genuinely
 *   doesn't exist there either, downstream queries return null and
 *   Room.svelte's redirect fires normally.
 *
 * Must be called during component initialization (uses Svelte context).
 */
export function useEffectiveSpaceId(getRoomId: () => string | undefined): {
  readonly current: string | null;
} {
  const roomsStore = getSpaceRoomsStore();
  const getActive = getActiveInstanceSpaceId();

  const current = $derived.by(() => {
    if (roomsStore.isInitialLoading) return null;
    const roomId = getRoomId();
    if (!roomId) return null;
    const matched = roomsStore.rooms.find((r) => r.id === roomId);
    if (matched?.type === RoomType.Dm) return DM_SPACE_ID;
    if (matched) return getActive();
    return DM_SPACE_ID;
  });

  return {
    get current() {
      return current;
    }
  };
}
