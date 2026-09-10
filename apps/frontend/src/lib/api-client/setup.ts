import { ServerSetupService } from '@chatto/api-types/chatto/auth/v1/setup_connect';
import { createPublicChattoClient } from './connect';

/** Complete first-run setup. Credentials remain transient and are never persisted here. */
export async function completeServerSetup(baseUrl: string, input: {
  serverName: string; description: string; login: string; displayName: string; password: string;
}): Promise<void> {
  await createPublicChattoClient(ServerSetupService, baseUrl).completeSetup(input);
}
