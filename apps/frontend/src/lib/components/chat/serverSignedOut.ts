import type { RegisteredServer } from '@chatto/client/server/registry';

/**
 * Whether the route of a remote server shows `ServerSignedOut` in place of
 * the server chrome.
 *
 * - A server without a session always shows the view.
 * - A server whose session the server rejected shows the view only while no
 *   chat data is loaded, for example after a page load. With retained data,
 *   the chrome stays visible and `AuthStatusNotice` offers to reconnect.
 *
 * The origin never shows the view: its route redirects to `/login`.
 */
export function showsServerSignedOut(
  server: Pick<RegisteredServer, 'token' | 'reauthRequiredAt'>,
  { isOrigin, hasDisplayableView }: { isOrigin: boolean; hasDisplayableView: boolean }
): boolean {
  if (isOrigin) return false;
  if (server.reauthRequiredAt !== null) return !hasDisplayableView;
  return server.token === null;
}
