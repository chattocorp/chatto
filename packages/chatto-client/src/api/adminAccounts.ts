/** Private account reads. Public profiles and assigned roles use UserService;
 * assignment limits use the role-management API. These reads keep no cache. */
import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';

type ReadOptions = { signal?: AbortSignal };

/** Create private account reads for servers that implement the focused RPCs.
 * Errors propagate; legacy member rows cannot fully represent these resources. */
export function createAdminAccountAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminUserService, config);
  return {
    /** Read verified addresses, verification times, and the primary choice.
     * Requires admin.view-users. An empty list means no verified addresses. */
    async listVerifiedEmails(userId: string, options: ReadOptions = {}) {
      return (await client.listVerifiedEmails({ userId }, options)).verifiedEmails;
    },
    /** Read username-change history with admin.view-users and either account
     * management authority or the caller's own ID. An absent timestamp means no history. */
    async getUsernameCooldown(userId: string, options: ReadOptions = {}) {
      const response = await client.getUsernameCooldown({ userId }, options);
      if (!response.cooldown) throw new Error('username cooldown response did not include history');
      return response.cooldown;
    }
  };
}
