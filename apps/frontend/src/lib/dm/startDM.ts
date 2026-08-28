import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { createRoomCommandAPI } from '$lib/api-client/rooms';

export type StartDirectMessageOptions = {
  /** Called with the direct-message room ID before navigation begins. */
  onRoomReady?: (roomId: string) => void;
};

/**
 * Start a DM conversation with a user and navigate to it.
 */
export async function startDMWith(
  serverId: string,
  userId: string,
  { onRoomReady }: StartDirectMessageOptions = {}
): Promise<void> {
  const conn = serverConnectionManager.getClient(serverId);
  const room = await conn.getAPI(createRoomCommandAPI).startDM([userId]);

  if (room) {
    onRoomReady?.(room.id);
    goto(
      resolve('/chat/[serverId]/[roomId]', {
        serverId: serverIdToSegment(serverId),
        roomId: room.id
      })
    );
  }
}
