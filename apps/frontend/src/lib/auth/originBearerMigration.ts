import { serverRegistry } from '$lib/state/server/registry.svelte';
import { browserCookieAuthenticationHeaders } from './authenticationMode';
import { csrfFetch } from './csrf';

/**
 * Revoke portable origin credentials before replacing them with cookie-only
 * browser authentication. Local state is left intact when revocation fails so
 * a later route load can retry without abandoning live bearer authority.
 */
export async function revokeLegacyOriginBearerSession(): Promise<void> {
  const origin = serverRegistry.originServer;
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
