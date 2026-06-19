import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { tryWireStartDM } from '$lib/wire';

/**
 * Start a DM conversation with a user and navigate to it.
 */
export async function startDMWith(serverId: string, userId: string): Promise<void> {
  const roomId = await tryWireStartDM(serverId, { participantIds: [userId] });
  if (!roomId) {
    throw new Error('wire client is not ready');
  }

  goto(
    resolve('/chat/[serverId]/[roomId]', {
      serverId: serverIdToSegment(serverId),
      roomId
    })
  );
}
