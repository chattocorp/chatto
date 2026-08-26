import { browserCookieAuthenticationHeaders } from './authenticationMode';

/**
 * Ask a 0.5 origin server to migrate the immediately previous signed browser
 * cookie. Returns false when no valid legacy authority remains.
 *
 * Deprecated: remove this 0.4-to-0.5 compatibility bridge in 0.6.
 */
export async function migrateLegacyOriginCookieSession(): Promise<boolean> {
  const response = await fetch('/auth/browser/session/migrate', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...browserCookieAuthenticationHeaders
    },
    body: '{}'
  });
  if (response.status === 204) return true;
  if (response.status === 401) return false;
  throw new Error(`Legacy browser-session migration failed (${response.status})`);
}
