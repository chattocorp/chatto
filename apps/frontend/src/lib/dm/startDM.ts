import { goto } from '$app/navigation';
import { userPath } from '$lib/navigation/conversationPath';

/**
 * Navigate to a recipient's DM destination. Its route load owns room creation.
 */
export async function startDMWith(serverId: string, userId: string): Promise<void> {
  await goto(userPath(serverId, userId));
}
