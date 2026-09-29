import type { ConnectAPIConfig } from '../api/connect.js';
import { getCurrentUserViaConnect, type CurrentUser } from '../api/viewer.js';
import { isAuthenticationRequiredError } from './errors.js';
import { revokeLegacyOriginBearerSession } from './originBearerMigration.js';
import { migrateLegacyOriginCookieSession } from './legacyCookieMigration.js';
import { isExplicitSignOutRedirectInProgress } from './signOut.js';

/** Read the origin account with cookie migration and one transient retry. Owns no account state. */
export async function getOriginViewer(config: ConnectAPIConfig): Promise<CurrentUser> {
  let migrationAttempted = false;
  let retried = false;
  while (true) {
    try {
      const user = await getCurrentUserViaConnect({ baseUrl: config.baseUrl, bearerToken: null });
      if (isExplicitSignOutRedirectInProgress()) return user;
      await revokeLegacyOriginBearerSession(config.serverId);
      return user;
    } catch (error) {
      if (isAuthenticationRequiredError(error)) {
        if (!migrationAttempted) {
          migrationAttempted = true;
          try {
            if (await migrateLegacyOriginCookieSession()) continue;
          } catch (migrationError) {
            if (retried) throw migrationError;
            retried = true;
            migrationAttempted = false;
            await new Promise((resolve) => setTimeout(resolve, 200));
            continue;
          }
        }
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
