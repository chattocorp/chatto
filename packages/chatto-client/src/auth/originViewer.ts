import type { ConnectAPIConfig } from '../api/connect.js';
import { getCurrentUserViaConnect, type CurrentUser } from '../api/viewer.js';
import { isAuthenticationRequiredError } from './errors.js';
import { revokeLegacyOriginBearerSession } from './originBearerMigration.js';
import { isExplicitSignOutRedirectInProgress } from './signOut.js';

/** Read the origin account with one transient retry. Owns no account state. */
export async function getOriginViewer(config: ConnectAPIConfig): Promise<CurrentUser> {
  let retried = false;
  while (true) {
    try {
      const user = await getCurrentUserViaConnect({ baseUrl: config.baseUrl, bearerToken: null });
      if (isExplicitSignOutRedirectInProgress()) return user;
      await revokeLegacyOriginBearerSession(config.serverId);
      return user;
    } catch (error) {
      if (isAuthenticationRequiredError(error)) {
        // Do not abandon portable authority when server-side revocation fails.
        await revokeLegacyOriginBearerSession(config.serverId);
        throw error;
      }
      if (retried) throw error;
      retried = true;
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
}
