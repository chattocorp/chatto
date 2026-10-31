import { serverConnectionManager, serverRegistry } from '$lib/client';
import { segmentToServerId } from '$lib/navigation';
import { userConversationRoomId } from '$lib/navigation/conversationPath';
import { createRoomCommandAPI } from '@chatto/client/api/rooms';
import { navigating } from '$app/state';
import type { LayoutLoad } from './$types';

/** Resolve only the recipient: thread navigation keeps the conversation and composer. */
export const load: LayoutLoad = async ({ params, parent, depends }) => {
  const committed = navigating.complete;
  await parent();
  depends('chatto:conversation');
  const serverId = segmentToServerId(params.serverId);
  const store = serverId ? serverRegistry.tryGetStore(serverId) : undefined;
  const userId = params.userId;

  const conversationRoomId = (async () => {
    try {
      // Start commands only after the destination commits, never during a canceled navigation.
      await committed;
      if (!serverId || !store?.accountId) return null;
      // Match the server layout's readiness boundary before reading its room projection.
      if (
        (!store.realtimeSync.hasDisplayableView || store.realtimeSync.isRecoveringSnapshot) &&
        !(await store.realtimeSync.waitForNextCaughtUp())
      )
        return null;
      let roomId = userConversationRoomId(store, userId);
      const known = roomId ? store.projection.rooms.get(roomId) : undefined;
      if (!known?.viewerState?.isMember || known.room?.archived) {
        const room = await serverConnectionManager
          .getClient(serverId)
          .getAPI(createRoomCommandAPI)
          .startDM(userId === store.accountId ? [] : [userId]);
        roomId = room?.id ?? null;
      }
      if (!roomId) throw new Error('Conversation is unavailable');
      await store.ensureRoomAvailable(roomId);
      return roomId;
    } catch {
      return null;
    }
  })();
  return { conversationRoomId };
};
