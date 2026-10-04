export const NOTIFICATION_CLICK_MESSAGE_TYPE = 'notification-click';
/**
 * Reply type that workers from earlier releases wait for. Current workers do
 * not wait for a reply; pages still send it so that an earlier worker does
 * not also open a new window.
 */
export const NOTIFICATION_CLICK_ACK_MESSAGE_TYPE = 'notification-click-ack';
const NOTIFICATION_CLICK_FALLBACK_PATH = '/chat';

/** The `WindowClient` members that click routing uses. */
export interface NotificationClickClient {
  url: string;
  focused: boolean;
  visibilityState: DocumentVisibilityState;
  focus(): Promise<unknown>;
  postMessage(message: unknown): void;
}

/** The `Clients` members that click routing uses. */
export interface NotificationClickClients {
  matchAll(options: {
    type: 'window';
    includeUncontrolled: true;
  }): Promise<readonly NotificationClickClient[]>;
  openWindow(url: string): Promise<unknown>;
}

function isChatPath(pathname: string): boolean {
  return pathname === '/chat' || pathname.startsWith('/chat/');
}

/**
 * Normalize a notification target to a URL on `origin`. A same-origin URL
 * stays as it is. A cross-origin chat URL keeps its path, query, and hash on
 * `origin`. Any other target becomes `/chat`.
 */
export function normalizeNotificationClickUrl(rawUrl: string | undefined, origin: string): string {
  const fallback = new URL(NOTIFICATION_CLICK_FALLBACK_PATH, origin).href;
  let url: URL;
  try {
    url = new URL(rawUrl ?? NOTIFICATION_CLICK_FALLBACK_PATH, origin);
  } catch {
    return fallback;
  }
  if (url.origin === origin) return url.href;
  if (isChatPath(url.pathname)) return new URL(url.pathname + url.search + url.hash, origin).href;
  return fallback;
}

/**
 * Path prefixes that the server never serves to the app, for example opened
 * attachments under `/assets`. Keep in sync with `isReservedNonFrontendPath`
 * in `cli/internal/http_server/frontend.go`.
 */
const NON_APP_PATH_PREFIXES = ['/api', '/auth', '/assets', '/.well-known'];

/**
 * Whether `client` runs the app and so can route a click. `Client.url` is the
 * URL that loaded the document; client-side navigation does not change it. A
 * window that the app loaded at `/` or `/login` is therefore an app window.
 */
function isAppWindow(client: NotificationClickClient): boolean {
  const { pathname } = new URL(client.url);
  return !NON_APP_PATH_PREFIXES.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)
  );
}

async function focusWindow(client: NotificationClickClient): Promise<boolean> {
  try {
    await client.focus();
    return true;
  } catch (err) {
    console.warn('[SW] Failed to focus existing window:', err);
    return false;
  }
}

/**
 * Route a notification click to `rawUrl` after normalizing it to this origin.
 *
 * The worker focuses the best open app window and sends it the target URL,
 * and the page routes in place. It prefers a focused window, then a visible
 * one, then any other. When no app window is open, or focus fails, the
 * worker opens the target in a new window.
 *
 * The worker that shows push notifications has a narrow `/__chatto/push/…/`
 * scope. It never controls the app windows, so `WindowClient.navigate()`
 * always fails there. Chromium allows one window action (`focus()` or
 * `openWindow()`) per click, so the worker focuses before it sends the message
 * and does not wait for a reply.
 */
export async function routeNotificationClick(
  rawUrl: string | undefined,
  origin: string,
  clients: NotificationClickClients
): Promise<void> {
  const url = normalizeNotificationClickUrl(rawUrl, origin);
  const windows = (await clients.matchAll({ type: 'window', includeUncontrolled: true })).filter(
    isAppWindow
  );
  const target =
    windows.find((client) => client.focused) ??
    windows.find((client) => client.visibilityState === 'visible') ??
    windows[0];

  if (target && (await focusWindow(target))) {
    try {
      target.postMessage({ type: NOTIFICATION_CLICK_MESSAGE_TYPE, url });
    } catch (err) {
      console.warn('[SW] Failed to send notification click to focused window:', err);
    }
    return;
  }

  await clients.openWindow(url);
}
