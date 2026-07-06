import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { createRoomCommandAPI } from '$lib/api-client/rooms';

/**
 * Start a DM conversation with a user and navigate to it.
 */
export async function startDMWith(serverId: string, userId: string): Promise<void> {
  const conn = serverConnectionManager.getClient(serverId);
  const room = await createRoomCommandAPI({
    serverId,
    baseUrl: conn.connectBaseUrl,
    bearerToken: conn.bearerToken
  }).startDM([userId]);

  if (room) {
    goto(
      resolve('/chat/[serverId]/[roomId]', {
        serverId: serverIdToSegment(serverId),
        roomId: room.id
      })
    );
  }
}
