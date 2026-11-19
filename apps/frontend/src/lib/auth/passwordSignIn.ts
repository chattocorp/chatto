import { browserCookieAuthenticationHeaders } from '@chatto/client/auth/authenticationMode';

/**
 * Sign in to the origin server with a password and set its session cookie.
 *
 * The caller completes the session with `completeOriginAuthentication`.
 * Throws an `Error` with the server message, or `fallbackMessage`, when the
 * server rejects the credentials. Network errors propagate unchanged.
 */
export async function signInWithPassword(
  identifier: string,
  password: string,
  fallbackMessage: string
): Promise<void> {
  const response = await fetch('/auth/browser/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...browserCookieAuthenticationHeaders
    },
    body: JSON.stringify({ identifier, password }),
    credentials: 'include'
  });
  if (response.ok) return;
  const result = await response.json().catch(() => ({}));
  throw new Error(result.error || fallbackMessage);
}
