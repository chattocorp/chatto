import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';

/**
 * Navigate to a recipient's DM destination. The mounted destination owns room creation.
 */
export async function startDMWith(
  serverId: string,
  userId: string
): Promise<void> {
  await goto(
    resolve('/chat/[serverId]/dm/[userId]', {
      serverId: serverIdToSegment(serverId),
      userId
    })
  );
}
