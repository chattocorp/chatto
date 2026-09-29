import { ownerOfServerId } from '../server/serverIds.js';
import { browserCookieAuthenticationHeaders } from './authenticationMode.js';
import { csrfFetch } from './csrf.js';

/**
 * Revoke portable origin credentials before replacing them with cookie-only
 * browser authentication. Local state is left intact when revocation fails so
 * a later route load can retry without abandoning live bearer authority.
 * `serverId` is the origin server's registry ID.
 */
export async function revokeLegacyOriginBearerSession(serverId: string | undefined): Promise<void> {
  const origin = serverId ? ownerOfServerId(serverId)?.getServer(serverId) : undefined;
  if (!origin?.token && !origin?.refreshToken) return;

  const response = await csrfFetch('/auth/browser/revoke-bearer-session', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...browserCookieAuthenticationHeaders
    },
    body: JSON.stringify({
      accessToken: origin.token,
      refreshToken: origin.refreshToken
    })
  });
  if (!response.ok) {
    throw new Error(`Origin bearer-session revocation failed (${response.status})`);
  }
}
