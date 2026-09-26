import type { ConnectAPIConfig } from '$lib/api-client/connect';
import { getCurrentUserViaConnect, type CurrentUser } from '$lib/api-client/viewer';
import { isAuthenticationRequiredError } from './errors';
import { revokeLegacyOriginBearerSession } from './originBearerMigration';
import { migrateLegacyOriginCookieSession } from './legacyCookieMigration';
import { isExplicitSignOutRedirectInProgress } from './signOut';

/** Read the origin account with cookie migration and one transient retry. Owns no account state. */
export async function getOriginViewer(config: ConnectAPIConfig): Promise<CurrentUser> {
  let migrationAttempted = false;
  let retried = false;
  while (true) {
    try {
      const user = await getCurrentUserViaConnect({ baseUrl: config.baseUrl, bearerToken: null });
      if (isExplicitSignOutRedirectInProgress()) return user;
      await revokeLegacyOriginBearerSession();
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
        await revokeLegacyOriginBearerSession();
        throw error;
      }
      if (retried) throw error;
      retried = true;
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
}
